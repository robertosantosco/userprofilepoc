package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
	"userprofilepoc/src/domain/entities"
	"userprofilepoc/src/helper/env"
	"userprofilepoc/src/infra/postgres"

	"github.com/go-faker/faker/v4"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SeederTemporalProperty usa string para o período, pois será processado pelo COPY TEXT.
type SeederTemporalProperty struct {
	EntityRef   string
	Key         string
	Value       json.RawMessage
	Period      string
	Granularity string
	StartTS     time.Time
}

type DataBundle struct {
	User          entities.Entity
	Org           entities.Entity
	Affiliations  []entities.Entity
	PosDevices    []entities.Entity
	TemporalProps []SeederTemporalProperty
}

// Estruturas para dados mais realistas
type MCC struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type PaymentMethodConfig struct {
	PaymentMethod string
	Network       *string
	Weight        float64
}

// Constantes para geração de dados
var (
	mccCodes = []MCC{
		{"5411", "Grocery Stores, Supermarkets"},
		{"5812", "Eating Places, Restaurants"},
		{"5541", "Service Stations"},
		{"5912", "Drug Stores, Pharmacies"},
		{"5651", "Family Clothing Stores"},
	}
	stoneDevices = []string{"Stone T2+", "Stone A920 Pro", "Stone T1", "Ton T2"}
)

func newSQLClient() (*pgxpool.Pool, error) {
	dbHost := env.MustGetString("DB_HOST")
	dbPort := env.GetString("DB_PORT", "5432")
	dbname := env.MustGetString("DB_NAME")
	dbUser := env.MustGetString("DB_USER")
	dbPassword := env.MustGetString("DB_PASSWORD")
	maxConnections := env.GetInt("DB_MAX_POOL_CONNECTIONS", 25)
	return postgres.NewPostgresClient(dbHost, dbPort, dbname, dbUser, dbPassword, maxConnections)
}

func main() {
	rand.Seed(time.Now().UnixNano())
	numClients := flag.Int("clients", 1, "Número de clientes a serem criados. Use -1 para infinito.")
	bulkSize := flag.Int("bulk-size", 100, "Quantidade de clientes a serem salvos em um único bulk insert.")
	monthsToGenerate := flag.Int("months", 6, "Quantos meses para trás gerar dados retroativos.")
	monthsPercentage := flag.Float64("months-perc", 100.0, "Percentual de meses a serem preenchidos (0-100).")
	usersPerOrg := flag.Int("users-per-org", 1, "Quantidade de usuários por organização.")
	numConsumers := flag.Int("consumers", 4, "Número de goroutines consumidoras.")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := newSQLClient()
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer db.Close()

	dataChan := make(chan DataBundle, *bulkSize**numConsumers)
	var wg sync.WaitGroup

	for i := 0; i < *numConsumers; i++ {
		wg.Add(1)
		go consumer(ctx, &wg, db, dataChan, *bulkSize, i+1)
	}

	wg.Add(1)
	go producer(ctx, &wg, dataChan, *numClients, *monthsToGenerate, *usersPerOrg, *monthsPercentage)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nShutdown signal received, stopping...")
		cancel()
	}()

	wg.Wait()
	fmt.Println("Seeding finished.")
}

func producer(ctx context.Context, wg *sync.WaitGroup, dataChan chan<- DataBundle, numClients, monthsToGenerate, usersPerOrg int, monthsPercentage float64) {
	defer wg.Done()
	defer close(dataChan)

	isInfinite := numClients == -1
	clientCount := 0

	for isInfinite || clientCount < numClients {
		select {
		case <-ctx.Done():
			fmt.Println("Producer stopping.")
			return
		default:
			org := generateFakeOrganization()
			for i := 0; i < usersPerOrg; i++ {
				if !isInfinite && clientCount >= numClients {
					break
				}
				user, affiliations, posDevices, temporalProps := generateFakeUserData(org, monthsToGenerate, monthsPercentage)
				dataChan <- DataBundle{
					User:          user,
					Org:           org,
					Affiliations:  affiliations,
					PosDevices:    posDevices,
					TemporalProps: temporalProps,
				}
				clientCount++
			}
		}
	}
}

func consumer(ctx context.Context, wg *sync.WaitGroup, db *pgxpool.Pool, dataChan <-chan DataBundle, bulkSize, consumerID int) {
	defer wg.Done()
	log.Printf("Consumer %d started", consumerID)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	bundles := make([]DataBundle, 0, bulkSize)

	for {
		select {
		case b, ok := <-dataChan:
			if !ok {
				if len(bundles) > 0 {
					log.Printf("Consumer %d: flushing %d bundles...", consumerID, len(bundles))
					if err := bulkInsert(ctx, db, bundles); err != nil {
						log.Printf("Consumer %d: ERROR on final flush: %v", consumerID, err)
					}
				}
				log.Printf("Consumer %d stopping.", consumerID)
				return
			}
			bundles = append(bundles, b)
			if len(bundles) >= bulkSize {
				if err := bulkInsert(ctx, db, bundles); err != nil {
					log.Printf("Consumer %d: ERROR on bulk insert: %v", consumerID, err)
				}
				bundles = make([]DataBundle, 0, bulkSize)
			}
		case <-ticker.C:
			if len(bundles) > 0 {
				if err := bulkInsert(ctx, db, bundles); err != nil {
					log.Printf("Consumer %d: ERROR on ticker flush: %v", consumerID, err)
				}
				bundles = make([]DataBundle, 0, bulkSize)
			}
		case <-ctx.Done():
			log.Printf("Consumer %d received stop signal.", consumerID)
			return
		}
	}
}

func bulkInsert(ctx context.Context, db *pgxpool.Pool, bundles []DataBundle) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	entityMap := make(map[string]int64)
	entityRows := [][]any{}
	allReferences := make([]string, 0, len(bundles)*4)
	refCheck := make(map[string]struct{})

	for _, b := range bundles {
		entitiesToAdd := []entities.Entity{b.User, b.Org}
		entitiesToAdd = append(entitiesToAdd, b.Affiliations...)
		entitiesToAdd = append(entitiesToAdd, b.PosDevices...)
		for _, e := range entitiesToAdd {
			entityRows = append(entityRows, []any{e.Type, e.Reference, e.Properties})
			if _, exists := refCheck[e.Reference]; !exists {
				allReferences = append(allReferences, e.Reference)
				refCheck[e.Reference] = struct{}{}
			}
		}
	}

	_, err = tx.CopyFrom(ctx, pgx.Identifier{"entities"}, []string{"type", "reference", "properties"}, pgx.CopyFromRows(entityRows))
	if err != nil {
		log.Printf("WARN: CopyFrom entities failed, likely due to conflicts. Proceeding to fetch IDs. Error: %v", err)
	}

	idRows, err := tx.Query(ctx, `SELECT id, reference FROM entities WHERE reference = ANY($1)`, allReferences)
	if err != nil {
		return fmt.Errorf("failed to re-fetch entity IDs: %w", err)
	}
	for idRows.Next() {
		var id int64
		var ref string
		if err := idRows.Scan(&id, &ref); err != nil {
			return err
		}
		entityMap[ref] = id
	}
	idRows.Close()

	edgeRows := [][]any{}
	for _, b := range bundles {
		userID, uok := entityMap[b.User.Reference]
		orgID, ook := entityMap[b.Org.Reference]
		if !uok || !ook {
			continue
		}
		edgeRows = append(edgeRows, []any{userID, orgID, "member_of"})
		for _, aff := range b.Affiliations {
			if affID, ok := entityMap[aff.Reference]; ok {
				edgeRows = append(edgeRows, []any{orgID, affID, "has_affiliation"})
			}
		}
	}
	if len(edgeRows) > 0 {
		_, err = tx.CopyFrom(ctx, pgx.Identifier{"edges"}, []string{"left_entity_id", "right_entity_id", "relationship_type"}, pgx.CopyFromRows(edgeRows))
		if err != nil {
			return fmt.Errorf("failed to bulk insert edges: %w", err)
		}
	}

	temporalRows := [][]any{}
	for _, b := range bundles {
		for _, p := range b.TemporalProps {
			if eid, ok := entityMap[p.EntityRef]; ok {
				temporalRows = append(temporalRows, []any{eid, p.Key, p.Value, p.Period, p.Granularity, p.StartTS})
			}
		}
	}
	if len(temporalRows) > 0 {
		if err := copyTemporalTEXT(ctx, tx, temporalRows); err != nil {
			log.Printf("bulkInsert temporal copy error: %v", err)
			return err
		}
	}

	return tx.Commit(ctx)
}

func copyTemporalTEXT(ctx context.Context, tx pgx.Tx, rows [][]any) error {
	var sb strings.Builder
	for _, r := range rows {
		eid := fmt.Sprintf("%v", r[0])
		key := toString(r[1])
		val := copyEscape(string(r[2].(json.RawMessage)))
		if !strings.HasPrefix(val, "{") {
			val = fmt.Sprintf("%s", val)
		}

		per := toString(r[3])
		gra := toString(r[4])
		ts := "1970-01-01 00:00:00"
		if t, ok := r[5].(time.Time); ok {
			ts = t.UTC().Format("2006-01-02 15:04:05-07")
		}
		sb.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\n",
			copyEscape(eid),
			copyEscape(key),
			val, // JSON não precisa de escape
			copyEscape(per),
			copyEscape(gra),
			copyEscape(ts)))
	}

	sql := `COPY temporal_properties (entity_id, key, value, period, granularity, start_ts) FROM STDIN WITH (FORMAT text)`
	_, err := tx.Conn().PgConn().CopyFrom(ctx, strings.NewReader(sb.String()), sql)
	return err
}

// ==== FUNÇÕES AUXILIARES ====
func getColumn(rows [][]any, col int) []any {
	out := make([]any, len(rows))
	for i := range rows {
		out[i] = rows[i][col]
	}
	return out
}
func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
func toJSONString(v any) string {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return fmt.Sprintf("%v", v)
}
func copyEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\t", "\\t")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// ==== FAKE DATA (LÓGICA ENRIQUECIDA) ====
func generateFakeOrganization() entities.Entity {
	mcc := mccCodes[rand.Intn(len(mccCodes))]
	companyName := faker.GetRealAddress().City + " " + faker.Word() + " " + faker.LastName()
	tradeName := companyName
	if rand.Float32() > 0.5 {
		tradeName = tradeName + " " + []string{"LTDA", "ME", "S.A."}[rand.Intn(3)]
	}

	props := map[string]interface{}{
		"company_name":           companyName,
		"trade_name":             tradeName,
		"document":               map[string]string{"type": "cnpj", "value": faker.Phonenumber()},
		"mcc":                    mcc,
		"estimated_monthly_tpv":  rand.Intn(2000000) + 50000,
		"merchant_status":        []string{"active", "inactive", "suspended"}[rand.Intn(3)],
		"approval_status":        []string{"approved", "pending", "under_review"}[rand.Intn(3)],
		"sales_channel":          []string{"direct_sales", "partner", "online"}[rand.Intn(3)],
		"risk_analysis_executed": rand.Float32() > 0.2,
	}
	propsBytes, _ := json.Marshal(props)
	return entities.Entity{Type: "organization", Reference: "acc-" + faker.UUIDHyphenated(), Properties: propsBytes}
}

func generateFakeUserData(org entities.Entity, monthsToGenerate int, monthsPercentage float64) (entities.Entity, []entities.Entity, []entities.Entity, []SeederTemporalProperty) {
	// 1. Gerar Usuário (PF)
	userProps := map[string]interface{}{
		"full_name":      faker.Name(),
		"email":          faker.Email(),
		"phone_number":   faker.Phonenumber(),
		"document":       map[string]string{"type": "cpf", "value": faker.Phonenumber()},
		"marital_status": []string{"solteiro", "casado", "divorciado", "viúvo"}[rand.Intn(4)],
		"nationality":    "Brasil",
		"address": map[string]string{
			"street":      faker.GetRealAddress().Address,
			"city":        faker.GetRealAddress().City,
			"state":       faker.GetRealAddress().State,
			"postal_code": faker.GetRealAddress().PostalCode,
		},
	}
	userPropsBytes, _ := json.Marshal(userProps)
	user := entities.Entity{Type: "user", Reference: "user-" + faker.UUIDHyphenated(), Properties: userPropsBytes}

	// 2. Gerar Afiliações e Dispositivos
	var affiliations, posDevices []entities.Entity
	numAffiliations := rand.Intn(2) + 1 // 1 ou 2 afiliações
	for i := 0; i < numAffiliations; i++ {
		affProps := map[string]interface{}{
			"stone_code":      fmt.Sprintf("%d", rand.Intn(999999999)),
			"status":          []string{"active", "inactive", "suspended"}[rand.Intn(3)],
			"payment_methods": []string{"pix", "visa", "mastercard", "elo"}[0 : rand.Intn(4)+1],
			"capture_methods": []string{"pos", "ecommerce", "link"}[0 : rand.Intn(3)+1],
			"monthly_volume":  rand.Intn(5000000) + 100000,
		}
		affPropsBytes, _ := json.Marshal(affProps)
		affiliation := entities.Entity{Type: "affiliation", Reference: "aff-" + faker.UUIDHyphenated(), Properties: affPropsBytes}
		affiliations = append(affiliations, affiliation)

		// Gerar POS para afiliação
		if monthlyVolume, ok := affProps["monthly_volume"].(int); ok && monthlyVolume > 500000 {
			numPos := rand.Intn(2) + 1
			for j := 0; j < numPos; j++ {
				posProps := map[string]string{
					"model":  stoneDevices[rand.Intn(len(stoneDevices))],
					"status": "active",
				}
				posPropsBytes, _ := json.Marshal(posProps)
				posDevices = append(posDevices, entities.Entity{Type: "pos_device", Reference: "pos-" + faker.UUIDHyphenated(), Properties: posPropsBytes})
			}
		}
	}

	// 3. Gerar Propriedades Temporais (com lógica de negócio)
	var temporalProps []SeederTemporalProperty
	now := time.Now().UTC()
	monthsToCreate := int(float64(monthsToGenerate) * (monthsPercentage / 100.0))

	// TPV Mensal para cada afiliação
	for _, aff := range affiliations {
		var affProps map[string]interface{}
		json.Unmarshal(aff.Properties, &affProps)
		baseTPV := affProps["monthly_volume"].(float64)

		for i := monthsToGenerate - 1; i >= monthsToGenerate-monthsToCreate; i-- {
			startOfMonth := time.Date(now.Year(), now.Month()-time.Month(i), 1, 0, 0, 0, 0, time.UTC)
			endOfMonth := startOfMonth.AddDate(0, 1, 0)

			// Fatores de crescimento e sazonalidade
			timeFactor := 1.0 - (float64(i)/float64(monthsToGenerate))*0.3 // Crescimento de 30% no período
			seasonalFactor := 1.0 + 0.2*math.Sin(float64(startOfMonth.Month())*math.Pi/6)
			monthTPV := baseTPV * timeFactor * seasonalFactor * (0.85 + rand.Float64()*0.3)

			// Evolução do PIX
			pixWeight := 0.15 + (0.2 * (float64(monthsToGenerate-i) / float64(monthsToGenerate))) // PIX cresce de 15% para 35%

			// Distribuição de pagamentos
			tpvItems := generateTPVItems(monthTPV, pixWeight)
			tpvValueBytes, _ := json.Marshal(map[string]interface{}{"items": tpvItems})

			period := fmt.Sprintf("[%s,%s)", startOfMonth.Format("2006-01-02 15:04:05-07"), endOfMonth.Format("2006-01-02 15:04:05-07"))
			temporalProps = append(temporalProps, SeederTemporalProperty{
				EntityRef:   aff.Reference,
				Key:         "tpv",
				Value:       tpvValueBytes,
				Period:      period,
				Granularity: "month",
				StartTS:     startOfMonth,
			})
		}
	}

	// Propriedades da Organização
	openPeriod := fmt.Sprintf("[%s,)", now.Format("2006-01-02 15:04:05-07"))
	balanceValue, _ := json.Marshal(map[string]interface{}{"available": rand.Intn(500000), "currency": "BRL"})
	creditLineValue, _ := json.Marshal(map[string]interface{}{"total_limit": rand.Intn(1000000), "available_limit": rand.Intn(500000)})
	kycStatusValue, _ := json.Marshal(map[string]interface{}{"approved": true, "verification_level": "complete", "last_updated": now.AddDate(0, -rand.Intn(6), -rand.Intn(28)).Format(time.RFC3339)})

	temporalProps = append(temporalProps,
		SeederTemporalProperty{EntityRef: org.Reference, Key: "account_balance", Value: balanceValue, Period: openPeriod, Granularity: "instant", StartTS: now},
		SeederTemporalProperty{EntityRef: org.Reference, Key: "credit_line", Value: creditLineValue, Period: openPeriod, Granularity: "instant", StartTS: now},
		SeederTemporalProperty{EntityRef: user.Reference, Key: "kyc_status", Value: kycStatusValue, Period: openPeriod, Granularity: "instant", StartTS: now},
	)

	return user, affiliations, posDevices, temporalProps
}

func generateTPVItems(monthTPV, pixWeight float64) []map[string]interface{} {
	var items []map[string]interface{}
	remainingTPV := monthTPV

	// PIX
	pixTPV := remainingTPV * pixWeight
	items = append(items, map[string]interface{}{
		"payment_method":    "pix",
		"amount":            int(pixTPV),
		"transaction_count": max(1, int(pixTPV/(float64(rand.Intn(150)+50)))),
		"mdr":               int(pixTPV * (0.005 + rand.Float64()*0.005)), // 0.5% a 1%
		"rav":               int(pixTPV * (0.01 + rand.Float64()*0.01)),   // 1% a 2%
	})
	remainingTPV -= pixTPV

	// Cartões
	creditWeight := 0.5 * (1 - pixWeight)
	debitWeight := 0.3 * (1 - pixWeight)
	voucherWeight := 0.1 * (1 - pixWeight)
	// boletoWeight := 0.1 * (1 - pixWeight)

	// Crédito
	creditTPV := remainingTPV * creditWeight
	items = append(items, map[string]interface{}{
		"payment_method":    "cartão de crédito",
		"network":           "visa",
		"amount":            int(creditTPV),
		"transaction_count": max(1, int(creditTPV/(float64(rand.Intn(200)+100)))),
		"mdr":               int(creditTPV * (0.018 + rand.Float64()*0.007)), // 1.8% a 2.5%
		"interchange":       int(creditTPV * (0.008 + rand.Float64()*0.004)), // 0.8% a 1.2%
	})
	remainingTPV -= creditTPV

	// Débito
	debitTPV := remainingTPV * debitWeight
	items = append(items, map[string]interface{}{
		"payment_method":    "cartão de débito",
		"network":           "mastercard",
		"amount":            int(debitTPV),
		"transaction_count": max(1, int(debitTPV/(float64(rand.Intn(100)+50)))),
		"mdr":               int(debitTPV * (0.008 + rand.Float64()*0.007)), // 0.8% a 1.5%
		"interchange":       int(debitTPV * (0.004 + rand.Float64()*0.004)), // 0.4% a 0.8%
	})
	remainingTPV -= debitTPV

	// Voucher
	voucherTPV := remainingTPV * voucherWeight
	items = append(items, map[string]interface{}{
		"payment_method":    "voucher",
		"network":           "alelo",
		"amount":            int(voucherTPV),
		"transaction_count": max(1, int(voucherTPV/(float64(rand.Intn(80)+40)))),
		"mdr":               int(voucherTPV * (0.025 + rand.Float64()*0.035)), // 2.5% a 6%
	})
	remainingTPV -= voucherTPV

	// Boleto
	if remainingTPV > 0 {
		items = append(items, map[string]interface{}{
			"payment_method":    "boleto",
			"amount":            int(remainingTPV),
			"transaction_count": max(1, int(remainingTPV/(float64(rand.Intn(500)+300)))),
		})
	}

	return items
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
