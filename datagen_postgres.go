//go:build datagen_postgres
// +build datagen_postgres

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
	"sync/atomic"
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
	EntityRef      string
	Key            string
	Value          json.RawMessage
	Granularity    string
	ReferenceDate  time.Time
	ReferenceMonth time.Time
	IdempotencyKey string
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
	dbHost := env.MustGetString("DB_WRITE_HOST")
	dbPort := env.GetString("DB_WRITE_PORT", "5432")
	dbname := env.MustGetString("DB_NAME")
	dbUser := env.MustGetString("DB_USER")
	dbPassword := env.MustGetString("DB_PASSWORD")
	maxConnections := 100
	return postgres.NewPostgresClient(dbHost, dbPort, dbname, dbUser, dbPassword, maxConnections)
}

func main() {
	rand.Seed(time.Now().UnixNano())

	// CONFIGURAÇÕES ULTRA AGRESSIVAS
	numClients := flag.Int("clients", -1, "Número de clientes a serem criados. Use -1 para infinito.")
	bulkSize := flag.Int("bulk-size", 1000, "BULK SIZE MASSIVO")
	monthsToGenerate := flag.Int("months", 3, "REDUZIR meses para acelerar")
	monthsPercentage := flag.Float64("months-perc", 50.0, "REDUZIR percentage")
	usersPerOrg := flag.Int("users-per-org", 1, "1 user por org para evitar conflitos")
	numConsumers := flag.Int("consumers", 16, "MUITO MAIS CONSUMERS")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := newSQLClient()
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer db.Close()

	// CHANNEL GIGANTE
	chanSize := (*bulkSize) * (*numConsumers) * 5
	dataChan := make(chan DataBundle, chanSize)

	var wg sync.WaitGroup
	var totalProcessed, totalErrors int64
	startTime := time.Now()

	// Métricas a cada 2 segundos
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				processed := atomic.LoadInt64(&totalProcessed)
				errors := atomic.LoadInt64(&totalErrors)
				elapsed := time.Since(startTime)
				rate := float64(processed) / elapsed.Seconds()

				fmt.Printf("📊 Processed: %d | Errors: %d | Rate: %.1f/s | Elapsed: %v\n",
					processed, errors, rate, elapsed.Round(time.Second))
			}
		}
	}()

	// INICIAR CONSUMERS
	for i := 0; i < *numConsumers; i++ {
		wg.Add(1)
		go optimizedConsumer(ctx, &wg, db, dataChan, *bulkSize, i+1, &totalProcessed, &totalErrors)
	}

	// INICIAR PRODUCER
	wg.Add(1)
	go producer(ctx, &wg, dataChan, *numClients, *monthsToGenerate, *usersPerOrg, *monthsPercentage)

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n🛑 Shutdown signal received, stopping...")
		cancel()
	}()

	wg.Wait()

	// Estatísticas finais
	elapsed := time.Since(startTime)
	processed := atomic.LoadInt64(&totalProcessed)
	errors := atomic.LoadInt64(&totalErrors)
	avgRate := float64(processed) / elapsed.Seconds()

	fmt.Printf("\n🏁 Seeding finished!\n")
	fmt.Printf("📊 Total processed: %d\n", processed)
	fmt.Printf("❌ Total errors: %d\n", errors)
	fmt.Printf("⏱️  Total time: %v\n", elapsed.Round(time.Second))
	fmt.Printf("🚀 Average rate: %.1f records/s\n", avgRate)
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
			// CORREÇÃO: Gerar uma organização por usuário para evitar conflitos
			// Ao invés de reutilizar a mesma org para múltiplos users

			for i := 0; i < usersPerOrg && (isInfinite || clientCount < numClients); i++ {
				// Gerar org ÚNICA para cada bundle
				org := generateFakeOrganization()
				user, affiliations, posDevices, temporalProps := generateFakeUserData(org, monthsToGenerate, monthsPercentage)

				select {
				case dataChan <- DataBundle{
					User:          user,
					Org:           org, // Org única para este bundle
					Affiliations:  affiliations,
					PosDevices:    posDevices,
					TemporalProps: temporalProps,
				}:
					clientCount++
					if clientCount%100 == 0 {
						fmt.Printf("Generated %d clients\n", clientCount)
					}
				case <-ctx.Done():
					return
				}
			}

			// Micro-pausa apenas para evitar 100% CPU
			if clientCount%1000 == 0 {
				time.Sleep(10 * time.Millisecond)
			}

			time.Sleep(25 * time.Millisecond)
		}
	}
}

func optimizedConsumer(ctx context.Context, wg *sync.WaitGroup, db *pgxpool.Pool, dataChan <-chan DataBundle, bulkSize, consumerID int, totalProcessed, totalErrors *int64) {
	defer wg.Done()
	log.Printf("🚀 Consumer %d started", consumerID)

	// Timeout menor para flush mais frequente
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	bundles := make([]DataBundle, 0, bulkSize)

	for {
		select {
		case b, ok := <-dataChan:
			if !ok {
				if len(bundles) > 0 {
					if err := bulkInsert(ctx, db, bundles); err != nil {
						log.Printf("❌ Consumer %d: ERROR on final flush: %v", consumerID, err)
						atomic.AddInt64(totalErrors, 1)
					} else {
						atomic.AddInt64(totalProcessed, int64(len(bundles)))
					}
				}
				log.Printf("✅ Consumer %d stopping.", consumerID)
				return
			}

			bundles = append(bundles, b)
			if len(bundles) >= bulkSize {
				if err := bulkInsert(ctx, db, bundles); err != nil {
					log.Printf("❌ Consumer %d: ERROR on bulk insert: %v", consumerID, err)
					atomic.AddInt64(totalErrors, 1)
				} else {
					atomic.AddInt64(totalProcessed, int64(len(bundles)))
				}
				bundles = make([]DataBundle, 0, bulkSize)
			}

		case <-ticker.C:
			if len(bundles) > 0 {
				if err := bulkInsert(ctx, db, bundles); err != nil {
					log.Printf("❌ Consumer %d: ERROR on ticker flush: %v", consumerID, err)
					atomic.AddInt64(totalErrors, 1)
				} else {
					atomic.AddInt64(totalProcessed, int64(len(bundles)))
				}
				bundles = make([]DataBundle, 0, bulkSize)
			}

		case <-ctx.Done():
			log.Printf("🛑 Consumer %d received stop signal.", consumerID)
			return
		}
	}
}

func bulkInsert(ctx context.Context, db *pgxpool.Pool, bundles []DataBundle) error {
	// Timeout mais longo para operações grandes
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	entityMap := make(map[string]int64, len(bundles)*5) // Pre-allocate
	allReferences := make([]string, 0, len(bundles)*5)
	refCheck := make(map[string]struct{}, len(bundles)*5)

	// Coletar entidades
	entityRows := make([][]any, 0, len(bundles)*5)
	for _, b := range bundles {
		entitiesToAdd := []entities.Entity{b.User, b.Org}
		entitiesToAdd = append(entitiesToAdd, b.Affiliations...)
		entitiesToAdd = append(entitiesToAdd, b.PosDevices...)
		for _, e := range entitiesToAdd {
			if _, exists := refCheck[e.Reference]; !exists {
				entityRows = append(entityRows, []any{e.Type, e.Reference, e.Properties})
				allReferences = append(allReferences, e.Reference)
				refCheck[e.Reference] = struct{}{}
			}
		}
	}

	// 1. INSERIR TODAS AS ENTIDADES DE UMA VEZ (SEM BATCHES)
	if len(entityRows) > 0 {
		types := make([]string, len(entityRows))
		references := make([]string, len(entityRows))
		properties := make([]string, len(entityRows))

		for i, row := range entityRows {
			types[i] = row[0].(string)
			references[i] = row[1].(string)
			if propBytes, ok := row[2].([]byte); ok {
				properties[i] = string(propBytes)
			} else {
				propJson, _ := json.Marshal(row[2])
				properties[i] = string(propJson)
			}
		}

		// INSERT MASSIVO
		insertSQL := `
			INSERT INTO entities (type, reference, properties) 
			SELECT unnest($1::text[]), unnest($2::text[]), unnest($3::jsonb[])
			ON CONFLICT (type, reference) DO NOTHING
		`

		_, err = tx.Exec(ctx, insertSQL, types, references, properties)
		if err != nil {
			return fmt.Errorf("failed to insert entities: %w", err)
		}
	}

	// 2. BUSCAR TODOS OS IDs DE UMA VEZ
	idRows, err := tx.Query(ctx, `SELECT id, reference FROM entities WHERE reference = ANY($1)`, allReferences)
	if err != nil {
		return fmt.Errorf("failed to fetch entity IDs: %w", err)
	}

	for idRows.Next() {
		var id int64
		var ref string
		if err := idRows.Scan(&id, &ref); err != nil {
			idRows.Close()
			return err
		}
		entityMap[ref] = id
	}
	idRows.Close()

	// 3. INSERIR TODAS AS EDGES DE UMA VEZ
	edgeRows := make([][]any, 0, len(bundles)*3)
	for _, b := range bundles {
		userID, uok := entityMap[b.User.Reference]
		orgID, ook := entityMap[b.Org.Reference]
		if uok && ook {
			edgeRows = append(edgeRows, []any{userID, orgID, "member_of"})
		}

		for _, aff := range b.Affiliations {
			if affID, ok := entityMap[aff.Reference]; ok && ook {
				edgeRows = append(edgeRows, []any{orgID, affID, "has_affiliation"})
			}
		}
	}

	if len(edgeRows) > 0 {
		leftIDs := make([]int64, len(edgeRows))
		rightIDs := make([]int64, len(edgeRows))
		relationshipTypes := make([]string, len(edgeRows))

		for i, edge := range edgeRows {
			leftIDs[i] = edge[0].(int64)
			rightIDs[i] = edge[1].(int64)
			relationshipTypes[i] = edge[2].(string)
		}

		edgeSQL := `
			INSERT INTO edges (left_entity_id, right_entity_id, relationship_type)
			SELECT unnest($1::bigint[]), unnest($2::bigint[]), unnest($3::text[])
			ON CONFLICT (left_entity_id, right_entity_id, relationship_type) DO NOTHING
		`

		_, err = tx.Exec(ctx, edgeSQL, leftIDs, rightIDs, relationshipTypes)
		if err != nil {
			return fmt.Errorf("failed to insert edges: %w", err)
		}
	}

	// 4. TEMPORAL PROPERTIES - INSERIR TUDO DE UMA VEZ
	temporalRows := make([][]any, 0, len(bundles)*5)
	for _, b := range bundles {
		for _, p := range b.TemporalProps {
			if eid, ok := entityMap[p.EntityRef]; ok {
				temporalRows = append(temporalRows, []any{
					eid, p.Key, p.Value, p.Granularity,
					p.ReferenceDate, p.ReferenceMonth,
					makeIdempotencyKey(eid, p.Key, p.Granularity, &p.ReferenceDate, &p.ReferenceMonth),
				})
			}
		}
	}

	if len(temporalRows) > 0 {
		entityIDs := make([]int64, len(temporalRows))
		keys := make([]string, len(temporalRows))
		values := make([]string, len(temporalRows))
		granularities := make([]string, len(temporalRows))
		referenceDates := make([]time.Time, len(temporalRows))
		referenceMonths := make([]time.Time, len(temporalRows))
		idempotencyKeys := make([]string, len(temporalRows))

		for i, r := range temporalRows {
			entityIDs[i] = r[0].(int64)
			keys[i] = toString(r[1])
			values[i] = string(r[2].(json.RawMessage))
			granularities[i] = toString(r[3])
			referenceDates[i] = r[4].(time.Time)
			referenceMonths[i] = r[5].(time.Time)
			idempotencyKeys[i] = toString(r[6])
		}

		insertSQL := `
			INSERT INTO temporal_properties (entity_id, key, value, granularity, reference_date, reference_month, idempotency_key) 
			SELECT unnest($1::bigint[]), unnest($2::text[]), unnest($3::jsonb[]), unnest($4::text[]), unnest($5::timestamptz[]), unnest($6::date[]), unnest($7::text[])
		`

		_, err := tx.Exec(ctx, insertSQL, entityIDs, keys, values, granularities, referenceDates, referenceMonths, idempotencyKeys)
		if err != nil {
			return fmt.Errorf("failed to insert temporal properties: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func copyTemporalOptimized(ctx context.Context, tx pgx.Tx, rows [][]any) error {
	// Usar buffer para melhor performance
	var buf strings.Builder
	buf.Grow(len(rows) * 200) // Pré-alocar memória

	for _, r := range rows {
		eid := fmt.Sprintf("%v", r[0])
		key := toString(r[1])
		val := string(r[2].(json.RawMessage))
		gran := toString(r[3])

		refDate := "\\N" // NULL em COPY format
		if r[4] != nil {
			refDate = r[4].(time.Time).Format("2006-01-02")
		}

		refMonth := "\\N"
		if r[5] != nil {
			refMonth = r[5].(time.Time).Format("2006-01-02")
		}

		idem := toString(r[6])

		// Usar fmt.Fprintf para melhor performance
		fmt.Fprintf(&buf, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			copyEscape(eid),
			copyEscape(key),
			copyEscape(val),
			copyEscape(gran),
			copyEscape(refDate),
			copyEscape(refMonth),
			copyEscape(idem))

	}

	sql := `COPY temporal_properties (entity_id, key, value, granularity, reference_date, reference_month, idempotency_key) FROM STDIN WITH (FORMAT text)`
	_, err := tx.Conn().PgConn().CopyFrom(ctx, strings.NewReader(buf.String()), sql)
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

		// Gerar POS para afiliação - CORRIGIDO
		var monthlyVolume float64
		switch v := affProps["monthly_volume"].(type) {
		case int:
			monthlyVolume = float64(v)
		case float64:
			monthlyVolume = v
		case int64:
			monthlyVolume = float64(v)
		default:
			monthlyVolume = 0
		}

		if monthlyVolume > 500000 {
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

	// 3. Gerar Propriedades Temporais (EVITANDO DUPLICATAS)
	var temporalProps []SeederTemporalProperty
	now := time.Now().UTC()
	monthsToCreate := int(float64(monthsToGenerate) * (monthsPercentage / 100.0))

	// Usar um map para evitar duplicatas
	generatedKeys := make(map[string]bool)

	// TPV Mensal para cada afiliação - CORRIGIDO
	for _, aff := range affiliations {
		var affProps map[string]interface{}
		json.Unmarshal(aff.Properties, &affProps)

		// CORREÇÃO: tratar tanto int quanto float64
		var baseTPV float64
		switch v := affProps["monthly_volume"].(type) {
		case int:
			baseTPV = float64(v)
		case float64:
			baseTPV = v
		case int64:
			baseTPV = float64(v)
		default:
			// fallback - tentar converter para float64
			if vol, ok := affProps["monthly_volume"]; ok {
				if volInt, err := json.Number(fmt.Sprintf("%v", vol)).Float64(); err == nil {
					baseTPV = volInt
				} else {
					baseTPV = 100000 // valor padrão se falhar
				}
			} else {
				baseTPV = 100000 // valor padrão
			}
		}

		for i := monthsToGenerate - 1; i >= monthsToGenerate-monthsToCreate; i-- {
			referenceMonth := time.Date(now.Year(), now.Month()-time.Month(i), 1, 0, 0, 0, 0, time.UTC)

			// Criar uma chave única para verificar duplicatas ANTES de gerar a entidade
			checkKey := fmt.Sprintf("%s:tpv:month:%s", aff.Reference, referenceMonth.Format("2006-01"))

			// PULAR se já foi gerado
			if generatedKeys[checkKey] {
				continue
			}
			generatedKeys[checkKey] = true

			// Fatores de crescimento e sazonalidade
			timeFactor := 1.0 - (float64(i)/float64(monthsToGenerate))*0.3 // Crescimento de 30% no período
			seasonalFactor := 1.0 + 0.2*math.Sin(float64(referenceMonth.Month())*math.Pi/6)
			monthTPV := baseTPV * timeFactor * seasonalFactor * (0.85 + rand.Float64()*0.3)

			// Evolução do PIX
			pixWeight := 0.15 + (0.2 * (float64(monthsToGenerate-i) / float64(monthsToGenerate))) // PIX cresce de 15% para 35%

			// Distribuição de pagamentos
			tpvItems := generateTPVItems(monthTPV, pixWeight)
			tpvValueBytes, _ := json.Marshal(map[string]interface{}{"items": tpvItems})

			// IMPORTANTE: usar dia fixo do mês para evitar variação na idempotency_key
			// Usar o último dia do mês para reference_date
			lastDay := time.Date(referenceMonth.Year(), referenceMonth.Month()+1, 0, 23, 59, 59, 0, time.UTC)

			temporalProps = append(temporalProps, SeederTemporalProperty{
				EntityRef:      aff.Reference,
				Key:            "tpv",
				Value:          tpvValueBytes,
				Granularity:    "month",
				ReferenceMonth: referenceMonth,
				ReferenceDate:  lastDay, // Data consistente
				IdempotencyKey: "",      // será gerado no makeIdempotencyKey
			})
		}
	}

	// Propriedades da Organização (GARANTINDO UNICIDADE)
	balanceValue, _ := json.Marshal(map[string]interface{}{"available": rand.Intn(500000), "currency": "BRL"})
	creditLineValue, _ := json.Marshal(map[string]interface{}{"total_limit": rand.Intn(1000000), "available_limit": rand.Intn(500000)})
	kycStatusValue, _ := json.Marshal(map[string]interface{}{"approved": true, "verification_level": "complete", "last_updated": now.AddDate(0, -rand.Intn(6), -rand.Intn(28)).Format(time.RFC3339)})

	currentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	// Verificar unicidade antes de adicionar
	orgBalanceKey := fmt.Sprintf("%s:account_balance:instant:%s", org.Reference, now.Format("2006-01-02T15:04:05.000"))
	orgCreditKey := fmt.Sprintf("%s:credit_line:instant:%s", org.Reference, now.Format("2006-01-02T15:04:05.000"))

	if !generatedKeys[orgBalanceKey] {
		generatedKeys[orgBalanceKey] = true
		temporalProps = append(temporalProps, SeederTemporalProperty{
			EntityRef:      org.Reference,
			Key:            "account_balance",
			Value:          balanceValue,
			Granularity:    "instant",
			ReferenceDate:  now,
			ReferenceMonth: currentMonth,
			IdempotencyKey: "",
		})
	}

	if !generatedKeys[orgCreditKey] {
		generatedKeys[orgCreditKey] = true
		temporalProps = append(temporalProps, SeederTemporalProperty{
			EntityRef:      org.Reference,
			Key:            "credit_line",
			Value:          creditLineValue,
			Granularity:    "instant",
			ReferenceDate:  now,
			ReferenceMonth: currentMonth,
			IdempotencyKey: "",
		})
	}

	// KYC para usuário (único por usuário)
	userKycKey := fmt.Sprintf("%s:kyc_status:instant:%s", user.Reference, now.Format("2006-01-02T15:04:05.000"))
	if !generatedKeys[userKycKey] {
		generatedKeys[userKycKey] = true
		temporalProps = append(temporalProps, SeederTemporalProperty{
			EntityRef:      user.Reference,
			Key:            "kyc_status",
			Value:          kycStatusValue,
			Granularity:    "instant",
			ReferenceDate:  now,
			ReferenceMonth: currentMonth,
			IdempotencyKey: "",
		})
	}

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

	// Boleto (restante)
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

func makeIdempotencyKey(entityID int64, key, granularity string, refDate, refMonth *time.Time) string {
	switch granularity {
	case "instant":
		// CORRIGIDO: usar milissegundos (.MS) igual ao SQL
		return fmt.Sprintf("%d:%s:%s:%s", entityID, key, granularity, refDate.UTC().Format("2006-01-02T15:04:05.000"))
	case "day":
		return fmt.Sprintf("%d:%s:%s:%s", entityID, key, granularity, refDate.UTC().Format("2006-01-02"))
	case "week":
		// CORRIGIDO: usar formato IYYY-WIW igual ao SQL
		year, week := refDate.UTC().ISOWeek()
		return fmt.Sprintf("%d:%s:%s:%d-W%02d", entityID, key, granularity, year, week)
	case "month":
		// CORRIGIDO: usar refMonth ao invés de refDate
		return fmt.Sprintf("%d:%s:%s:%s", entityID, key, granularity, refMonth.UTC().Format("2006-01"))
	case "quarter":
		// CORRIGIDO: calcular quarter corretamente
		q := (int(refDate.Month())-1)/3 + 1
		return fmt.Sprintf("%d:%s:%s:%d-Q%d", entityID, key, granularity, refDate.Year(), q)
	case "year":
		return fmt.Sprintf("%d:%s:%s:%d", entityID, key, granularity, refDate.Year())
	}
	return ""
}
