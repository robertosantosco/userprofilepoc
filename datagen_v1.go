package main

// import (
// 	"context"
// 	"encoding/json"
// 	"flag"
// 	"fmt"
// 	"log"
// 	"math/rand"
// 	"os"
// 	"os/signal"
// 	"strings"
// 	"sync"
// 	"syscall"
// 	"time"
// 	"userprofilepoc/src/domain/entities"
// 	"userprofilepoc/src/helper/env"
// 	"userprofilepoc/src/infra/postgres"

// 	"github.com/go-faker/faker/v4"
// 	"github.com/jackc/pgx/v5"
// 	"github.com/jackc/pgx/v5/pgxpool"
// )

// type SeederTemporalProperty struct {
// 	EntityRef   string
// 	Key         string
// 	Value       json.RawMessage
// 	Period      string
// 	Granularity string
// 	StartTS     time.Time
// }

// type DataBundle struct {
// 	User          entities.Entity
// 	Org           entities.Entity
// 	Affiliations  []entities.Entity
// 	PosDevices    []entities.Entity
// 	TemporalProps []SeederTemporalProperty
// }

// func newSQLClient() (*pgxpool.Pool, error) {
// 	dbHost := env.MustGetString("DB_HOST")
// 	dbPort := env.GetString("DB_PORT", "5432")
// 	dbname := env.MustGetString("DB_NAME")
// 	dbUser := env.MustGetString("DB_USER")
// 	dbPassword := env.MustGetString("DB_PASSWORD")
// 	maxConnections := env.GetInt("DB_MAX_POOL_CONNECTIONS", 25)
// 	return postgres.NewPostgresClient(dbHost, dbPort, dbname, dbUser, dbPassword, maxConnections)
// }

// func main() {
// 	numClients := flag.Int("clients", 1, "Número de clientes a serem criados. Use -1 para infinito.")
// 	bulkSize := flag.Int("bulk-size", 100, "Quantidade de clientes a serem salvos em um único bulk insert.")
// 	monthsToGenerate := flag.Int("months", 6, "Quantos meses para trás gerar dados retroativos.")
// 	monthsPercentage := flag.Float64("months-perc", 100.0, "Percentual de meses a serem preenchidos (0-100).")
// 	usersPerOrg := flag.Int("users-per-org", 1, "Quantidade de usuários por organização.")
// 	numConsumers := flag.Int("consumers", 4, "Número de goroutines consumidoras.")
// 	flag.Parse()

// 	ctx, cancel := context.WithCancel(context.Background())
// 	defer cancel()

// 	db, err := newSQLClient()
// 	if err != nil {
// 		log.Fatalf("Failed to connect: %v", err)
// 	}
// 	defer db.Close()

// 	dataChan := make(chan DataBundle, *bulkSize**numConsumers)
// 	var wg sync.WaitGroup

// 	for i := 0; i < *numConsumers; i++ {
// 		wg.Add(1)
// 		go consumer(ctx, &wg, db, dataChan, *bulkSize, i+1)
// 	}

// 	wg.Add(1)
// 	go producer(ctx, &wg, dataChan, *numClients, *monthsToGenerate, *usersPerOrg, *monthsPercentage)

// 	sigChan := make(chan os.Signal, 1)
// 	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
// 	go func() {
// 		<-sigChan
// 		fmt.Println("\nShutdown signal received, stopping...")
// 		cancel()
// 	}()

// 	wg.Wait()
// 	fmt.Println("Seeding finished.")
// }

// func producer(ctx context.Context, wg *sync.WaitGroup, dataChan chan<- DataBundle, numClients, monthsToGenerate, usersPerOrg int, monthsPercentage float64) {
// 	defer wg.Done()
// 	defer close(dataChan)

// 	isInfinite := numClients == -1
// 	clientCount := 0

// 	for isInfinite || clientCount < numClients {
// 		select {
// 		case <-ctx.Done():
// 			fmt.Println("Producer stopping.")
// 			return
// 		default:
// 			org := generateFakeOrganization()
// 			for i := 0; i < usersPerOrg; i++ {
// 				if !isInfinite && clientCount >= numClients {
// 					break
// 				}
// 				user, affiliations, posDevices, temporalProps := generateFakeUserData(org, monthsToGenerate, monthsPercentage)
// 				dataChan <- DataBundle{
// 					User:          user,
// 					Org:           org,
// 					Affiliations:  affiliations,
// 					PosDevices:    posDevices,
// 					TemporalProps: temporalProps,
// 				}
// 				clientCount++
// 			}
// 		}
// 	}
// }

// func consumer(ctx context.Context, wg *sync.WaitGroup, db *pgxpool.Pool, dataChan <-chan DataBundle, bulkSize, consumerID int) {
// 	defer wg.Done()
// 	log.Printf("Consumer %d started", consumerID)

// 	ticker := time.NewTicker(5 * time.Second)
// 	defer ticker.Stop()
// 	bundles := make([]DataBundle, 0, bulkSize)

// 	for {
// 		select {
// 		case b, ok := <-dataChan:
// 			if !ok {
// 				if len(bundles) > 0 {
// 					log.Printf("Consumer %d: flushing %d bundles...", consumerID, len(bundles))
// 					if err := bulkInsert(ctx, db, bundles); err != nil {
// 						log.Printf("Consumer %d: ERROR on final flush: %v", consumerID, err)
// 					}
// 				}
// 				log.Printf("Consumer %d stopping.", consumerID)
// 				return
// 			}
// 			bundles = append(bundles, b)
// 			if len(bundles) >= bulkSize {
// 				if err := bulkInsert(ctx, db, bundles); err != nil {
// 					log.Printf("Consumer %d: ERROR on bulk insert: %v", consumerID, err)
// 				}
// 				bundles = make([]DataBundle, 0, bulkSize)
// 			}
// 		case <-ticker.C:
// 			if len(bundles) > 0 {
// 				if err := bulkInsert(ctx, db, bundles); err != nil {
// 					log.Printf("Consumer %d: ERROR on ticker flush: %v", consumerID, err)
// 				}
// 				bundles = make([]DataBundle, 0, bulkSize)
// 			}
// 		case <-ctx.Done():
// 			log.Printf("Consumer %d received stop signal.", consumerID)
// 			return
// 		}
// 	}
// }

// func bulkInsert(ctx context.Context, db *pgxpool.Pool, bundles []DataBundle) error {
// 	tx, err := db.Begin(ctx)
// 	if err != nil {
// 		return fmt.Errorf("failed to begin transaction: %w", err)
// 	}
// 	defer tx.Rollback(ctx)

// 	entityMap := make(map[string]int64)
// 	entityRows := [][]any{}
// 	allReferences := make([]string, 0, len(bundles)*4)
// 	refCheck := make(map[string]struct{})

// 	for _, b := range bundles {
// 		entitiesToAdd := []entities.Entity{b.User, b.Org}
// 		entitiesToAdd = append(entitiesToAdd, b.Affiliations...)
// 		entitiesToAdd = append(entitiesToAdd, b.PosDevices...)
// 		for _, e := range entitiesToAdd {
// 			entityRows = append(entityRows, []any{e.Type, e.Reference, e.Properties})
// 			if _, exists := refCheck[e.Reference]; !exists {
// 				allReferences = append(allReferences, e.Reference)
// 				refCheck[e.Reference] = struct{}{}
// 			}
// 		}
// 	}

// 	_, err = tx.CopyFrom(ctx, pgx.Identifier{"entities"}, []string{"type", "reference", "properties"}, pgx.CopyFromRows(entityRows))
// 	if err != nil {
// 		log.Printf("WARN: CopyFrom entities failed, likely due to conflicts. Proceeding to fetch IDs. Error: %v", err)
// 	}

// 	idRows, err := tx.Query(ctx, `SELECT id, reference FROM entities WHERE reference = ANY($1)`, allReferences)
// 	if err != nil {
// 		return fmt.Errorf("failed to re-fetch entity IDs: %w", err)
// 	}
// 	for idRows.Next() {
// 		var id int64
// 		var ref string
// 		if err := idRows.Scan(&id, &ref); err != nil {
// 			return err
// 		}
// 		entityMap[ref] = id
// 	}
// 	idRows.Close()

// 	edgeRows := [][]any{}
// 	for _, b := range bundles {
// 		userID, uok := entityMap[b.User.Reference]
// 		orgID, ook := entityMap[b.Org.Reference]
// 		if !uok || !ook {
// 			continue
// 		}
// 		edgeRows = append(edgeRows, []any{userID, orgID, "member_of"})
// 		for _, aff := range b.Affiliations {
// 			if affID, ok := entityMap[aff.Reference]; ok {
// 				edgeRows = append(edgeRows, []any{orgID, affID, "has_affiliation"})
// 			}
// 		}
// 	}
// 	if len(edgeRows) > 0 {
// 		_, err = tx.CopyFrom(ctx, pgx.Identifier{"edges"}, []string{"left_entity_id", "right_entity_id", "relationship_type"}, pgx.CopyFromRows(edgeRows))
// 		if err != nil {
// 			return fmt.Errorf("failed to bulk insert edges: %w", err)
// 		}
// 	}

// 	temporalRows := [][]any{}
// 	for _, b := range bundles {
// 		for _, p := range b.TemporalProps {
// 			if eid, ok := entityMap[p.EntityRef]; ok {
// 				temporalRows = append(temporalRows, []any{eid, p.Key, p.Value, p.Period, p.Granularity, p.StartTS})
// 			}
// 		}
// 	}
// 	if len(temporalRows) > 0 {
// 		if err := copyTemporalTEXT(ctx, tx, temporalRows); err != nil {
// 			log.Printf("bulkInsert temporal copy error: %v", err)
// 			return err
// 		}
// 	}

// 	return tx.Commit(ctx)
// }

// func copyTemporalTEXT(ctx context.Context, tx pgx.Tx, rows [][]any) error {
// 	var sb strings.Builder
// 	for _, r := range rows {
// 		eid := fmt.Sprintf("%v", r[0])
// 		key := toString(r[1])
// 		b, _ := json.Marshal(r[2])
// 		val := copyEscape(string(b))
// 		// garantir que esteja entre aspas
// 		if !strings.HasPrefix(val, "{") {
// 			val = fmt.Sprintf("%s", val)
// 		}
// 		per := toString(r[3])
// 		gra := toString(r[4])
// 		ts := "1970-01-01 00:00:00"
// 		if t, ok := r[5].(time.Time); ok {
// 			ts = t.UTC().Format("2006-01-02 15:04:05-07")
// 		}

// 		sb.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\n",
// 			copyEscape(eid),
// 			copyEscape(key),
// 			val,
// 			copyEscape(per),
// 			copyEscape(gra),
// 			copyEscape(ts)))
// 	}

// 	sql := `COPY temporal_properties (entity_id, key, value, period, granularity, start_ts) FROM STDIN WITH (FORMAT text)`
// 	_, err := tx.Conn().PgConn().CopyFrom(ctx, strings.NewReader(sb.String()), sql)
// 	return err
// }

// // ==== FUNÇÕES AUXILIARES ====
// func getColumn(rows [][]any, col int) []any {
// 	out := make([]any, len(rows))
// 	for i := range rows {
// 		out[i] = rows[i][col]
// 	}
// 	return out
// }
// func toString(v any) string {
// 	if s, ok := v.(string); ok {
// 		return s
// 	}
// 	return fmt.Sprintf("%v", v)
// }
// func toJSONString(v any) string {
// 	if b, ok := v.([]byte); ok {
// 		return string(b)
// 	}
// 	return fmt.Sprintf("%v", v)
// }
// func copyEscape(s string) string {
// 	s = strings.ReplaceAll(s, "\\", "\\\\")
// 	s = strings.ReplaceAll(s, "\t", "\\t")
// 	s = strings.ReplaceAll(s, "\n", "\\n")
// 	return s
// }

// // ==== FAKE DATA ====
// func generateFakeOrganization() entities.Entity {
// 	orgProps := fmt.Sprintf(`{"company_name":"%s","trade_name":"%s","document":{"type":"cnpj","value":"%s"}}`,
// 		faker.Name()+" LTDA", faker.Name(), faker.Phonenumber())
// 	return entities.Entity{Type: "organization", Reference: "acc-" + faker.UUIDHyphenated(), Properties: json.RawMessage(orgProps)}
// }
// func generateFakeUserData(org entities.Entity, monthsToGenerate int, monthsPercentage float64) (entities.Entity, []entities.Entity, []entities.Entity, []SeederTemporalProperty) {
// 	userProps := fmt.Sprintf(`{"full_name":"%s","email":"%s","phone_number":"%s","document":{"type":"cpf","value":"%s"}}`,
// 		faker.Name(), faker.Email(), faker.Phonenumber(), faker.Phonenumber())
// 	user := entities.Entity{Type: "user", Reference: "user-" + faker.UUIDHyphenated(), Properties: json.RawMessage(userProps)}

// 	affiliations := make([]entities.Entity, 2)
// 	for i := 0; i < 2; i++ {
// 		affProps := fmt.Sprintf(`{"stone_code":%d,"status":"%s"}`,
// 			rand.Intn(999999999), []string{"active", "inactive"}[rand.Intn(2)])
// 		affiliations[i] = entities.Entity{Type: "affiliation", Reference: "aff-" + faker.UUIDHyphenated(), Properties: json.RawMessage(affProps)}
// 	}

// 	posDevices := make([]entities.Entity, 1)
// 	posDevices[0] = entities.Entity{Type: "pos_device", Reference: "pos-" + faker.UUIDHyphenated(), Properties: json.RawMessage(`{"model":"Stone A920","status":"active"}`)}

// 	temporalProps := []SeederTemporalProperty{}
// 	now := time.Now().UTC()
// 	monthsToCreate := int(float64(monthsToGenerate) * (monthsPercentage / 100.0))

// 	for i := monthsToGenerate - 1; i >= monthsToGenerate-monthsToCreate; i-- {
// 		startOfMonth := time.Date(now.Year(), now.Month()-time.Month(i), 1, 0, 0, 0, 0, time.UTC)
// 		endOfMonth := startOfMonth.AddDate(0, 1, 0)

// 		for _, aff := range affiliations {
// 			period := fmt.Sprintf("[%s,%s)",
// 				startOfMonth.Format("2006-01-02 15:04:05-07"),
// 				endOfMonth.Format("2006-01-02 15:04:05-07"))
// 			temporalProps = append(temporalProps, SeederTemporalProperty{
// 				EntityRef:   aff.Reference,
// 				Key:         "tpv",
// 				Value:       json.RawMessage(`{"amount":1000}`),
// 				Period:      period,
// 				Granularity: "month",
// 				StartTS:     startOfMonth,
// 			})
// 		}
// 	}

// 	openPeriod := fmt.Sprintf("[%s,)", now.Format("2006-01-02 15:04:05-07"))
// 	temporalProps = append(temporalProps,
// 		SeederTemporalProperty{EntityRef: org.Reference, Key: "account_balance", Value: json.RawMessage(`{"available":5000}`), Period: openPeriod, Granularity: "instant", StartTS: now},
// 		SeederTemporalProperty{EntityRef: org.Reference, Key: "credit_line", Value: json.RawMessage(`{"limit":10000}`), Period: openPeriod, Granularity: "instant", StartTS: now},
// 	)

// 	return user, affiliations, posDevices, temporalProps
// }
