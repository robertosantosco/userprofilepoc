//go:build datagen_properties
// +build datagen_properties

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
	"syscall"
	"time"
	"userprofilepoc/src/infra/kafka"

	"github.com/go-faker/faker/v4"
)

// PropertyMessage representa o schema da mensagem Kafka para propriedades
type PropertyMessage struct {
	EntityReference string `json:"entity_reference"`
	EntityType      string `json:"entity_type"`
	FieldName       string `json:"field_name"`
	FieldValue      string `json:"field_value"`
	ReferenceDate   string `json:"reference_date"`
}

// Entity types and their fields
var entityConfigs = map[string][]string{
	"user": {"email", "name", "phone", "age", "department", "position", "location"},
	"product": {"name", "price", "category", "description", "sku", "brand", "status"},
	"organization": {"name", "industry", "size", "location", "founded_year", "revenue", "employees"},
	"device": {"model", "serial_number", "status", "last_seen", "version", "location"},
	"store": {"name", "address", "manager", "opening_hours", "phone", "category"},
}

// Sample data for realistic generation
var sampleData = map[string]map[string][]string{
	"user": {
		"email":      {"@pagar.me", "@gmail.com", "@hotmail.com", "@company.com", "@startup.io"},
		"department": {"Engineering", "Sales", "Marketing", "Support", "Finance", "HR"},
		"position":   {"Developer", "Manager", "Analyst", "Director", "Coordinator", "Specialist"},
		"location":   {"São Paulo", "Rio de Janeiro", "Belo Horizonte", "Remote", "Brasília"},
	},
	"product": {
		"category": {"Electronics", "Clothing", "Books", "Home", "Sports", "Beauty"},
		"brand":    {"Apple", "Samsung", "Nike", "Adidas", "Sony", "Microsoft"},
		"status":   {"active", "inactive", "discontinued", "pending"},
	},
	"organization": {
		"industry": {"Technology", "Finance", "Healthcare", "Retail", "Education", "Manufacturing"},
		"size":     {"Startup", "Small", "Medium", "Large", "Enterprise"},
		"location": {"São Paulo", "Rio de Janeiro", "Belo Horizonte", "Porto Alegre", "Recife"},
	},
	"device": {
		"model":  {"iPhone 15", "Galaxy S24", "MacBook Pro", "ThinkPad X1", "iPad Pro"},
		"status": {"online", "offline", "maintenance", "retired"},
	},
	"store": {
		"category":      {"Supermarket", "Electronics", "Clothing", "Restaurant", "Pharmacy"},
		"opening_hours": {"08:00-18:00", "09:00-19:00", "24h", "10:00-22:00", "07:00-20:00"},
	},
}

// generatePropertyValue generates realistic values for entity properties
func generatePropertyValue(entityType, fieldName string) string {
	if samples, exists := sampleData[entityType][fieldName]; exists {
		return samples[rand.Intn(len(samples))]
	}

	// Fallback generation based on field name
	switch fieldName {
	case "email":
		return faker.Email()
	case "name":
		return faker.Name()
	case "phone":
		return "+55" + fmt.Sprintf("%d", rand.Intn(89999999999)+10000000000)
	case "age":
		return fmt.Sprintf("%d", rand.Intn(50)+18)
	case "price":
		return fmt.Sprintf("%.2f", rand.Float64()*1000+10)
	case "description":
		return faker.Sentence()
	case "sku":
		return fmt.Sprintf("SKU-%d", rand.Intn(999999)+100000)
	case "serial_number":
		return fmt.Sprintf("SN%d%s", rand.Intn(999999), faker.Word())
	case "founded_year":
		return fmt.Sprintf("%d", rand.Intn(30)+1990)
	case "revenue":
		return fmt.Sprintf("%.0f", rand.Float64()*10000000+100000)
	case "employees":
		return fmt.Sprintf("%d", rand.Intn(10000)+1)
	case "last_seen":
		return time.Now().Add(-time.Duration(rand.Intn(72)) * time.Hour).Format("2006-01-02 15:04:05")
	case "version":
		return fmt.Sprintf("v%d.%d.%d", rand.Intn(5)+1, rand.Intn(10), rand.Intn(10))
	case "address":
		return faker.GetRealAddress().Address + ", " + faker.GetRealAddress().City
	case "manager":
		return faker.Name()
	default:
		return faker.Word()
	}
}

// generateEntityReference creates a unique reference for an entity
func generateEntityReference(entityType string, index int) string {
	return fmt.Sprintf("%s_%s_%d", entityType, faker.Username(), index)
}

// generateEntityProperties creates properties for a single entity
func generateEntityProperties(entityRef, entityType string, baseTime time.Time, conflictRate float64) []PropertyMessage {
	var properties []PropertyMessage

	fields := entityConfigs[entityType]
	numFields := rand.Intn(3) + 2 // 2-4 fields per entity

	// Select random fields for this entity
	selectedFields := make([]string, 0, numFields)
	fieldIndices := rand.Perm(len(fields))
	for i := 0; i < numFields && i < len(fieldIndices); i++ {
		selectedFields = append(selectedFields, fields[fieldIndices[i]])
	}

	for _, fieldName := range selectedFields {
		// Generate initial property
		value := generatePropertyValue(entityType, fieldName)
		timestamp := baseTime.Add(time.Duration(rand.Intn(7200)) * time.Second) // ±1 hour variation

		prop := PropertyMessage{
			EntityReference: entityRef,
			EntityType:      entityType,
			FieldName:       fieldName,
			FieldValue:      value,
			ReferenceDate:   timestamp.Format("2006-01-02 15:04:05"),
		}
		properties = append(properties, prop)

		// Generate conflict (update) with probability
		if rand.Float64() < conflictRate {
			conflictValue := generatePropertyValue(entityType, fieldName)
			conflictTime := timestamp.Add(time.Duration(rand.Intn(3600)+300) * time.Second) // 5 minutes to 1 hour later

			conflictProp := PropertyMessage{
				EntityReference: entityRef,
				EntityType:      entityType,
				FieldName:       fieldName,
				FieldValue:      conflictValue,
				ReferenceDate:   conflictTime.Format("2006-01-02 15:04:05"),
			}
			properties = append(properties, conflictProp)
		}
	}

	return properties
}

// generateBatch creates a batch of property messages
func generateBatch(totalMessages int, conflictRate float64) []PropertyMessage {
	var messages []PropertyMessage
	baseTime := time.Now().UTC()

	entityTypes := make([]string, 0, len(entityConfigs))
	for entityType := range entityConfigs {
		entityTypes = append(entityTypes, entityType)
	}

	// Calculate number of entities (entities will have multiple properties)
	avgPropsPerEntity := 3.5 // Average properties per entity
	numEntities := int(float64(totalMessages) / avgPropsPerEntity)
	if numEntities < 1 {
		numEntities = 1
	}

	entityIndex := 0
	for len(messages) < totalMessages && entityIndex < numEntities*2 { // Safety multiplier
		entityType := entityTypes[rand.Intn(len(entityTypes))]
		entityRef := generateEntityReference(entityType, entityIndex)

		// Generate properties for this entity
		entityProps := generateEntityProperties(entityRef, entityType, baseTime, conflictRate)

		// Add to messages batch
		for _, prop := range entityProps {
			if len(messages) < totalMessages {
				messages = append(messages, prop)
			}
		}

		entityIndex++
	}

	// Shuffle messages to mix entities and properties
	for i := range messages {
		j := rand.Intn(i + 1)
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages[:min(len(messages), totalMessages)]
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
	totalMessages := flag.Int("count", 1000, "Total number of property messages to generate. Use -1 for infinite.")
	batchSize := flag.Int("batch-size", 100, "Number of messages per batch")
	topic := flag.String("topic", "", "Kafka topic to send messages to (required)")
	brokers := flag.String("brokers", "", "Kafka brokers (comma-separated) (required)")
	groupID := flag.String("group-id", "", "Kafka group ID (required)")
	delayMs := flag.Int("delay-ms", 100, "Delay in milliseconds between batches")
	conflictRate := flag.Float64("conflict-rate", 0.2, "Probability of conflicts per property (0.0-1.0)")
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

	isInfinite := *totalMessages == -1
	if isInfinite {
		log.Printf("Starting properties datagen in INFINITE mode with batches of %d (conflict rate: %.1f%%)",
			*batchSize, *conflictRate*100)
	} else {
		log.Printf("Starting properties datagen with %d messages in batches of %d (conflict rate: %.1f%%)",
			*totalMessages, *batchSize, *conflictRate*100)
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
	entitiesGenerated := make(map[string]bool)
	conflictsGenerated := 0
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
		batch := generateBatch(currentBatchSize, *conflictRate)

		// Count entities and conflicts for stats
		batchEntities := make(map[string]bool)
		batchConflicts := 0

		for _, msg := range batch {
			entityKey := msg.EntityReference + "|" + msg.EntityType
			batchEntities[entityKey] = true
			entitiesGenerated[entityKey] = true

			// Simple conflict detection (same entity+field seen before in this batch)
			// This is approximate since we don't track across batches
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
		conflictsGenerated += batchConflicts

		// Progress logging
		if messagesSent%500 == 0 || (!isInfinite && messagesSent >= *totalMessages) {
			elapsed := time.Since(startTime)
			rate := float64(messagesSent) / elapsed.Seconds()
			if isInfinite {
				log.Printf("Sent %d messages, %d entities (%.1f msg/sec)",
					messagesSent, len(entitiesGenerated), rate)
			} else {
				log.Printf("Sent %d/%d messages, %d entities (%.1f msg/sec)",
					messagesSent, *totalMessages, len(entitiesGenerated), rate)
			}
		}

		// Delay between batches
		if *delayMs > 0 && (isInfinite || messagesSent < *totalMessages) {
			time.Sleep(time.Duration(*delayMs) * time.Millisecond)
		}
	}

	elapsed := time.Since(startTime)
	rate := float64(messagesSent) / elapsed.Seconds()

	log.Printf("\n📊 Generation Stats:")
	log.Printf("Messages sent: %d", messagesSent)
	log.Printf("Unique entities: %d", len(entitiesGenerated))
	log.Printf("Avg properties per entity: %.1f", float64(messagesSent)/float64(len(entitiesGenerated)))
	log.Printf("Total time: %v", elapsed)
	log.Printf("Rate: %.1f msg/sec", rate)

	if isInfinite {
		log.Printf("✅ Stopped! Generated realistic property data")
	} else {
		log.Printf("✅ Completed! Generated realistic property data")
	}
}