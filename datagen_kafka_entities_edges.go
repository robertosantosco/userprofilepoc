//go:build datagen_kafka_entities_edges
// +build datagen_kafka_entities_edges

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

// KafkaEntityMessage represents the Kafka message schema
type KafkaEntityMessage struct {
	ID        string      `json:"id"`
	Reference string      `json:"reference"`
	Type      string      `json:"type"`
	Edges     []KafkaEdge `json:"edges"`
}

type KafkaEdge struct {
	EntityReference string `json:"entity_reference"`
	EntityType      string `json:"entity_type"`
	RelationType    string `json:"relation_type"`
}

// Entity types and relation types
var (
	entityTypes   = []string{"user", "organization", "product", "device", "store"}
	relationTypes = []string{"affiliation", "ownership", "membership", "purchase", "manages"}
)

// generateEntityReference creates a unique reference for an entity
func generateEntityReference(entityType string) string {
	return fmt.Sprintf("%s_%s_%d", entityType, faker.Username(), rand.Intn(1000000))
}

// generateKafkaMessage creates a realistic message with random edges
func generateKafkaMessage() KafkaEntityMessage {
	mainType := entityTypes[rand.Intn(len(entityTypes))]
	mainRef := generateEntityReference(mainType)

	// Generate 0-5 random edges
	numEdges := rand.Intn(6)
	edges := make([]KafkaEdge, numEdges)

	for i := 0; i < numEdges; i++ {
		targetType := entityTypes[rand.Intn(len(entityTypes))]
		// Ensure target is different from source
		for targetType == mainType {
			targetType = entityTypes[rand.Intn(len(entityTypes))]
		}

		edges[i] = KafkaEdge{
			EntityReference: generateEntityReference(targetType),
			EntityType:      targetType,
			RelationType:    relationTypes[rand.Intn(len(relationTypes))],
		}
	}

	return KafkaEntityMessage{
		ID:        faker.UUIDHyphenated(),
		Reference: mainRef,
		Type:      mainType,
		Edges:     edges,
	}
}

// generateBatch creates a batch of messages for more realistic scenarios
func generateBatch(size int) []KafkaEntityMessage {
	messages := make([]KafkaEntityMessage, size)

	// Track references to create realistic cross-references
	referencesPool := make(map[string]string) // reference -> type

	for i := 0; i < size; i++ {
		mainType := entityTypes[rand.Intn(len(entityTypes))]
		mainRef := generateEntityReference(mainType)
		referencesPool[mainRef] = mainType

		var edges []KafkaEdge

		// 30% chance to reference existing entities (creates realistic interconnections)
		if len(referencesPool) > 1 && rand.Float32() < 0.3 {
			for existingRef, existingType := range referencesPool {
				if existingRef != mainRef && len(edges) < 3 { // Max 3 existing refs
					edges = append(edges, KafkaEdge{
						EntityReference: existingRef,
						EntityType:      existingType,
						RelationType:    relationTypes[rand.Intn(len(relationTypes))],
					})
				}
			}
		}

		// Add some new references (simulates entities that will arrive later)
		newEdgesCount := rand.Intn(4) // 0-3 new edges
		for j := 0; j < newEdgesCount; j++ {
			targetType := entityTypes[rand.Intn(len(entityTypes))]
			newRef := generateEntityReference(targetType)

			edges = append(edges, KafkaEdge{
				EntityReference: newRef,
				EntityType:      targetType,
				RelationType:    relationTypes[rand.Intn(len(relationTypes))],
			})

			// Add to pool for future references
			referencesPool[newRef] = targetType
		}

		messages[i] = KafkaEntityMessage{
			ID:        faker.UUIDHyphenated(),
			Reference: mainRef,
			Type:      mainType,
			Edges:     edges,
		}
	}

	return messages
}

func main() {
	rand.Seed(time.Now().UnixNano())

	// Command line flags
	totalMessages := flag.Int("count", 1000, "Total number of messages to generate. Use -1 for infinite.")
	batchSize := flag.Int("batch-size", 100, "Number of messages per batch")
	topic := flag.String("topic", "", "Kafka topic to send messages to (required)")
	brokers := flag.String("brokers", "", "Kafka brokers (comma-separated) (required)")
	groupID := flag.String("group-id", "", "Kafka group ID (required)")
	delayMs := flag.Int("delay-ms", 100, "Delay in milliseconds between batches")
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
		log.Printf("Starting Kafka datagen in INFINITE mode with batches of %d", *batchSize)
	} else {
		log.Printf("Starting Kafka datagen with %d messages in batches of %d", *totalMessages, *batchSize)
	}

	// Create Kafka client
	kafkaClient, err := kafka.NewKafkaClient(*brokers, *groupID, 5000)
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
		batch := generateBatch(currentBatchSize)

		// Convert to Kafka messages
		kafkaMessages := make([]kafka.Message, len(batch))
		for i, msg := range batch {
			msgBytes, err := json.Marshal(msg)
			if err != nil {
				log.Printf("Failed to marshal message: %v", err)
				continue
			}

			kafkaMessages[i] = kafka.Message{
				Key:   msg.Reference, // Use entity reference as key for partitioning
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
		if messagesSent%500 == 0 || (!isInfinite && messagesSent == *totalMessages) {
			elapsed := time.Since(startTime)
			rate := float64(messagesSent) / elapsed.Seconds()
			if isInfinite {
				log.Printf("Sent %d messages (%.1f msg/sec)", messagesSent, rate)
			} else {
				log.Printf("Sent %d/%d messages (%.1f msg/sec)", messagesSent, *totalMessages, rate)
			}
		}

		// Delay between batches
		if *delayMs > 0 && (isInfinite || messagesSent < *totalMessages) {
			time.Sleep(time.Duration(*delayMs) * time.Millisecond)
		}
	}

	elapsed := time.Since(startTime)
	rate := float64(messagesSent) / elapsed.Seconds()
	if isInfinite {
		log.Printf("✅ Stopped! Sent %d messages in %v (%.1f msg/sec)", messagesSent, elapsed, rate)
	} else {
		log.Printf("✅ Completed! Sent %d messages in %v (%.1f msg/sec)", messagesSent, elapsed, rate)
	}
}
