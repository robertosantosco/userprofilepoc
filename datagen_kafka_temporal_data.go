//go:build datagen_temporal
// +build datagen_temporal

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"userprofilepoc/src/infra/kafka"

	"github.com/go-faker/faker/v4"
)

// TemporalMessage representa o schema da mensagem Kafka para dados temporais
type TemporalMessage struct {
	EntityReference       string      `json:"entity_reference"`
	EntityType            string      `json:"entity_type"`
	PropertyType          string      `json:"property_type"`
	PropertyValue         interface{} `json:"property_value"`
	PropertyReferenceDate string      `json:"property_reference_date"`
	PropertyGranularity   string      `json:"property_granularity"`
}

// Entity types and their temporal metrics
var temporalMetrics = map[string][]string{
	"organization": {"tpv", "revenue", "transaction_count", "active_users", "churn_rate"},
	"user":         {"session_duration", "login_frequency", "feature_usage", "engagement_score"},
	"product":      {"sales_metrics", "inventory_levels", "conversion_rate", "rating_average"},
}

// Value generators for each metric type
func generateMetricValue(entityType, propertyType string, granularity string) interface{} {
	baseMultiplier := getGranularityMultiplier(granularity)

	switch propertyType {
	// Organization metrics
	case "tpv":
		return map[string]interface{}{
			"amount":           rand.Float64()*500000*baseMultiplier + 10000,
			"currency":         "BRL",
			"transaction_count": int(rand.Float64()*2000*baseMultiplier) + 50,
		}
	case "revenue":
		return map[string]interface{}{
			"amount":      rand.Float64()*100000*baseMultiplier + 5000,
			"currency":    "BRL",
			"margin":      rand.Float64()*0.3 + 0.1, // 10-40% margin
			"cost_amount": rand.Float64()*70000*baseMultiplier + 3000,
		}
	case "transaction_count":
		return map[string]interface{}{
			"total":       int(rand.Float64()*1500*baseMultiplier) + 20,
			"successful":  int(rand.Float64()*1400*baseMultiplier) + 18,
			"failed":      int(rand.Float64()*100*baseMultiplier) + 2,
			"avg_amount":  rand.Float64()*500 + 50,
		}
	case "active_users":
		return map[string]interface{}{
			"count":          int(rand.Float64()*5000*baseMultiplier) + 100,
			"new_users":      int(rand.Float64()*200*baseMultiplier) + 5,
			"returning_users": int(rand.Float64()*4800*baseMultiplier) + 95,
		}
	case "churn_rate":
		return map[string]interface{}{
			"rate":         rand.Float64()*0.15 + 0.01, // 1-16% churn
			"churned_users": int(rand.Float64()*50*baseMultiplier) + 1,
			"total_users":   int(rand.Float64()*1000*baseMultiplier) + 100,
		}

	// User metrics
	case "session_duration":
		return map[string]interface{}{
			"total_minutes": int(rand.Float64()*180*baseMultiplier) + 5, // 5-185 minutes
			"page_views":    int(rand.Float64()*50*baseMultiplier) + 1,
			"actions_count": int(rand.Float64()*30*baseMultiplier) + 1,
			"bounce_rate":   rand.Float64()*0.4 + 0.1, // 10-50% bounce
		}
	case "login_frequency":
		return map[string]interface{}{
			"logins_count": int(rand.Float64()*20*baseMultiplier) + 1,
			"unique_days":  int(rand.Float64()*10*baseMultiplier) + 1,
			"avg_session":  rand.Float64()*120 + 10, // 10-130 minutes
		}
	case "feature_usage":
		return map[string]interface{}{
			"features_used":    int(rand.Float64()*15) + 1,
			"premium_features": int(rand.Float64()*5),
			"time_spent":       int(rand.Float64()*300*baseMultiplier) + 10,
		}
	case "engagement_score":
		return map[string]interface{}{
			"score":       rand.Float64()*100 + 10, // 10-110 score
			"interactions": int(rand.Float64()*100*baseMultiplier) + 5,
			"content_views": int(rand.Float64()*200*baseMultiplier) + 10,
		}

	// Product metrics
	case "sales_metrics":
		return map[string]interface{}{
			"units_sold": int(rand.Float64()*200*baseMultiplier) + 1,
			"revenue":    rand.Float64()*10000*baseMultiplier + 100,
			"returns":    int(rand.Float64()*10*baseMultiplier),
			"avg_price":  rand.Float64()*500 + 25,
		}
	case "inventory_levels":
		return map[string]interface{}{
			"current_stock": int(rand.Float64()*1000) + 10,
			"reserved":      int(rand.Float64()*50) + 1,
			"available":     int(rand.Float64()*950) + 9,
			"reorder_point": int(rand.Float64()*100) + 20,
		}
	case "conversion_rate":
		return map[string]interface{}{
			"rate":      rand.Float64()*0.15 + 0.01, // 1-16% conversion
			"views":     int(rand.Float64()*5000*baseMultiplier) + 100,
			"purchases": int(rand.Float64()*300*baseMultiplier) + 5,
		}
	case "rating_average":
		return map[string]interface{}{
			"average":     rand.Float64()*2 + 3,   // 3-5 stars
			"total_votes": int(rand.Float64()*500*baseMultiplier) + 5,
			"five_star":   int(rand.Float64()*300*baseMultiplier) + 3,
			"one_star":    int(rand.Float64()*50*baseMultiplier),
		}

	default:
		return map[string]interface{}{
			"value": rand.Float64() * 1000,
			"count": int(rand.Float64() * 100),
		}
	}
}

// getGranularityMultiplier returns multiplier based on time period
func getGranularityMultiplier(granularity string) float64 {
	switch granularity {
	case "hour":
		return 1.0
	case "day":
		return 24.0
	case "week":
		return 168.0 // 24 * 7
	case "month":
		return 720.0 // 24 * 30
	default:
		return 1.0
	}
}

// generateEntityReference creates a realistic entity reference
func generateEntityReference(entityType string, index int) string {
	switch entityType {
	case "organization":
		return fmt.Sprintf("acc-%s", faker.UUIDDigit())[:16]
	case "user":
		return fmt.Sprintf("user-%s", faker.Username())
	case "product":
		return fmt.Sprintf("prod-%s", strings.ReplaceAll(faker.UUIDDigit(), "-", ""))[:12]
	default:
		return fmt.Sprintf("%s-%d", entityType, index)
	}
}

// generateTimestampForGranularity creates proper timestamps aligned to granularity
func generateTimestampForGranularity(baseTime time.Time, granularity string, periodsBack int) time.Time {
	switch granularity {
	case "hour":
		// Align to hour boundary and go back N hours
		aligned := baseTime.Truncate(time.Hour)
		return aligned.Add(-time.Duration(periodsBack) * time.Hour)
	case "day":
		// Align to day boundary (midnight UTC) and go back N days
		aligned := time.Date(baseTime.Year(), baseTime.Month(), baseTime.Day(), 0, 0, 0, 0, time.UTC)
		return aligned.AddDate(0, 0, -periodsBack)
	case "week":
		// Align to Monday and go back N weeks
		weekday := int(baseTime.Weekday())
		if weekday == 0 {
			weekday = 7 // Sunday = 7
		}
		daysToMonday := weekday - 1
		monday := time.Date(baseTime.Year(), baseTime.Month(), baseTime.Day()-daysToMonday, 0, 0, 0, 0, time.UTC)
		return monday.AddDate(0, 0, -periodsBack*7)
	case "month":
		// Align to first day of month and go back N months
		firstDay := time.Date(baseTime.Year(), baseTime.Month(), 1, 0, 0, 0, 0, time.UTC)
		return firstDay.AddDate(0, -periodsBack, 0)
	default:
		return baseTime.Add(-time.Duration(periodsBack) * time.Hour)
	}
}

// generateTimeSeriesForEntity creates a complete time series for one entity metric
func generateTimeSeriesForEntity(entityRef, entityType, propertyType, granularity string, periodsCount int, baseTime time.Time) []TemporalMessage {
	var messages []TemporalMessage

	for i := 0; i < periodsCount; i++ {
		timestamp := generateTimestampForGranularity(baseTime, granularity, i)
		value := generateMetricValue(entityType, propertyType, granularity)

		message := TemporalMessage{
			EntityReference:       entityRef,
			EntityType:            entityType,
			PropertyType:          propertyType,
			PropertyValue:         value,
			PropertyReferenceDate: timestamp.Format(time.RFC3339),
			PropertyGranularity:   granularity,
		}

		messages = append(messages, message)
	}

	return messages
}

// generateBatch creates a batch of temporal messages
func generateBatch(batchSize int, entities []string, granularities []string, periodsBack int) []TemporalMessage {
	var messages []TemporalMessage
	baseTime := time.Now().UTC()

	entityTypes := make([]string, 0, len(temporalMetrics))
	for entityType := range temporalMetrics {
		entityTypes = append(entityTypes, entityType)
	}

	for len(messages) < batchSize {
		// Select random entity type and its metrics
		entityType := entityTypes[rand.Intn(len(entityTypes))]
		metrics := temporalMetrics[entityType]

		// Select or generate entity reference
		var entityRef string
		if len(entities) > 0 {
			entityRef = entities[rand.Intn(len(entities))]
		} else {
			entityRef = generateEntityReference(entityType, rand.Intn(1000))
		}

		// Select random metric and granularity
		propertyType := metrics[rand.Intn(len(metrics))]
		granularity := granularities[rand.Intn(len(granularities))]

		// Generate time series (1-5 periods for this batch)
		periodsCount := rand.Intn(5) + 1
		if periodsCount > periodsBack {
			periodsCount = periodsBack
		}

		timeSeries := generateTimeSeriesForEntity(entityRef, entityType, propertyType, granularity, periodsCount, baseTime)

		// Add to messages
		for _, msg := range timeSeries {
			if len(messages) < batchSize {
				messages = append(messages, msg)
			}
		}
	}

	// Shuffle to mix different entities/metrics
	for i := range messages {
		j := rand.Intn(i + 1)
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages[:min(len(messages), batchSize)]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	rand.Seed(time.Now().UnixNano())

	// Command line flags
	totalMessages := flag.Int("count", 1000, "Total number of temporal messages to generate. Use -1 for infinite.")
	batchSize := flag.Int("batch-size", 100, "Number of messages per batch")
	topic := flag.String("topic", "", "Kafka topic to send messages to (required)")
	brokers := flag.String("brokers", "", "Kafka brokers (comma-separated) (required)")
	groupID := flag.String("group-id", "", "Kafka group ID (required)")
	delayMs := flag.Int("delay-ms", 100, "Delay in milliseconds between batches")
	entitiesCount := flag.Int("entities", 50, "Number of unique entities to generate")
	daysBack := flag.Int("days-back", 30, "How many periods back to generate data")
	granularitiesFlag := flag.String("granularities", "day,month", "Granularities to generate (comma-separated: hour,day,week,month)")
	flag.Parse()

	// Validate required flags
	if *topic == "" {
		log.Fatal("The 'topic' flag is required")
	}
	if *brokers == "" {
		log.Fatal("The 'brokers' flag is required")
	}
	if *groupID == "" {
		log.Fatal("The 'group-id' flag is required")
	}

	// Parse granularities
	granularities := strings.Split(*granularitiesFlag, ",")
	for i, g := range granularities {
		granularities[i] = strings.TrimSpace(g)
	}

	// Generate entity references pool
	var entities []string
	entityTypes := []string{"organization", "user", "product"}
	for i := 0; i < *entitiesCount; i++ {
		entityType := entityTypes[rand.Intn(len(entityTypes))]
		entityRef := generateEntityReference(entityType, i)
		entities = append(entities, entityRef)
	}

	isInfinite := *totalMessages == -1
	if isInfinite {
		log.Printf("Starting temporal datagen in INFINITE mode with batches of %d", *batchSize)
		log.Printf("Entities: %d, Granularities: %v, Days back: %d", *entitiesCount, granularities, *daysBack)
	} else {
		log.Printf("Starting temporal datagen with %d messages in batches of %d", *totalMessages, *batchSize)
		log.Printf("Entities: %d, Granularities: %v, Days back: %d", *entitiesCount, granularities, *daysBack)
	}

	// Create Kafka client
	kafkaClient, err := kafka.NewKafkaClient(*brokers, *groupID, *batchSize)
	if err != nil {
		log.Fatalf("Failed to create Kafka client: %v", err)
	}
	defer kafkaClient.Close()

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received shutdown signal, stopping...")
		cancel()
	}()

	// Generate and send messages
	messagesSent := 0
	uniqueEntities := make(map[string]bool)
	startTime := time.Now()

	for isInfinite || messagesSent < *totalMessages {
		select {
		case <-ctx.Done():
			log.Println("Shutdown requested, stopping message generation")
			return
		default:
		}

		// Calculate batch size for this iteration
		currentBatchSize := *batchSize
		if !isInfinite {
			remainingMessages := *totalMessages - messagesSent
			if remainingMessages < currentBatchSize {
				currentBatchSize = remainingMessages
			}
		}

		// Generate batch
		batch := generateBatch(currentBatchSize, entities, granularities, *daysBack)

		// Track unique entities for stats
		for _, msg := range batch {
			entityKey := msg.EntityReference + "|" + msg.EntityType
			uniqueEntities[entityKey] = true
		}

		// Convert to Kafka messages
		kafkaMessages := make([]kafka.Message, len(batch))
		for i, msg := range batch {
			msgBytes, err := json.Marshal(msg)
			if err != nil {
				log.Printf("Failed to marshal message: %v", err)
				continue
			}

			kafkaMessages[i] = kafka.Message{
				Key:   msg.EntityReference, // Use entity reference as key for partitioning
				Value: msgBytes,
			}
		}

		// Send to Kafka
		if err := kafkaClient.Producer(kafkaMessages, *topic); err != nil {
			log.Printf("Failed to send batch: %v", err)
			continue
		}

		messagesSent += len(batch)

		// Progress logging
		if messagesSent%500 == 0 || (!isInfinite && messagesSent >= *totalMessages) {
			elapsed := time.Since(startTime)
			rate := float64(messagesSent) / elapsed.Seconds()
			if isInfinite {
				log.Printf("Sent %d temporal messages, %d entities (%.1f msg/sec)",
					messagesSent, len(uniqueEntities), rate)
			} else {
				log.Printf("Sent %d/%d temporal messages, %d entities (%.1f msg/sec)",
					messagesSent, *totalMessages, len(uniqueEntities), rate)
			}
		}

		// Delay between batches
		if *delayMs > 0 && (isInfinite || messagesSent < *totalMessages) {
			time.Sleep(time.Duration(*delayMs) * time.Millisecond)
		}
	}

	elapsed := time.Since(startTime)
	rate := float64(messagesSent) / elapsed.Seconds()

	log.Printf("\n📊 Temporal Data Generation Stats:")
	log.Printf("Messages sent: %d", messagesSent)
	log.Printf("Unique entities: %d", len(uniqueEntities))
	log.Printf("Granularities: %v", granularities)
	log.Printf("Time series periods: %d", *daysBack)
	log.Printf("Total time: %v", elapsed)
	log.Printf("Rate: %.1f msg/sec", rate)

	if isInfinite {
		log.Printf("✅ Stopped! Generated realistic temporal data")
	} else {
		log.Printf("✅ Completed! Generated realistic temporal data")
	}
}