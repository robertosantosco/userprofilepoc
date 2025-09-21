package consumers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"userprofilepoc/src/domain"
	"userprofilepoc/src/infra/kafka"
	"userprofilepoc/src/repositories"
)

// KafkaEntityMessage representa o schema da mensagem Kafka
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

type EntitiesEdgesConsumer struct {
	logger               *slog.Logger
	graphWriteRepository *repositories.GraphWriteRepository
}

func NewEntitiesEdgesConsumer(
	logger *slog.Logger,
	graphWriteRepository *repositories.GraphWriteRepository,
) *EntitiesEdgesConsumer {
	return &EntitiesEdgesConsumer{
		logger:               logger,
		graphWriteRepository: graphWriteRepository,
	}
}

func (c *EntitiesEdgesConsumer) Start(ctx context.Context, kafkaClient *kafka.KafkaClient, topic string) error {
	c.logger.Info("Starting entities/edges consumer", "topic", topic)

	handler := func(messages []kafka.Message) error {
		return c.handleMessages(ctx, messages)
	}

	return kafkaClient.Consumer(ctx, handler, topic)
}

func (c *EntitiesEdgesConsumer) handleMessages(ctx context.Context, messages []kafka.Message) error {
	if len(messages) == 0 {
		return nil
	}

	c.logger.Info("Processing messages batch", "count", len(messages))

	// Track all entities to avoid duplicates (reference -> entity)
	entitiesMap := make(map[string]domain.SyncEntityDTO)
	var relationships []domain.SyncRelationshipDTO

	for _, msg := range messages {
		var kafkaEntityMessage KafkaEntityMessage
		if err := json.Unmarshal(msg.Value, &kafkaEntityMessage); err != nil {
			c.logger.Error("Failed to unmarshal message",
				"error", err,
				"key", msg.Key,
				"value", string(msg.Value))
			return fmt.Errorf("failed to unmarshal message with key %s: %w", msg.Key, err)
		}

		// Validate required fields
		if kafkaEntityMessage.Reference == "" || kafkaEntityMessage.Type == "" {
			c.logger.Error("Invalid message: missing required fields",
				"key", msg.Key,
				"reference", kafkaEntityMessage.Reference,
				"type", kafkaEntityMessage.Type)
			return fmt.Errorf("invalid message with key %s: reference and type are required", msg.Key)
		}

		// Create main entity (from the message itself)
		mainEntity := domain.SyncEntityDTO{
			Reference: kafkaEntityMessage.Reference,
			Type:      kafkaEntityMessage.Type,
		}
		entitiesMap[kafkaEntityMessage.Reference] = mainEntity

		// Process edges: create referenced entities and relationships
		for _, edge := range kafkaEntityMessage.Edges {
			if edge.EntityReference == "" || edge.EntityType == "" || edge.RelationType == "" {
				c.logger.Warn("Skipping edge with missing fields",
					"sourceReference", kafkaEntityMessage.Reference,
					"targetReference", edge.EntityReference,
					"targetType", edge.EntityType,
					"relationType", edge.RelationType,
				)
				continue
			}

			// Create referenced entity if it doesn't exist yet
			if _, exists := entitiesMap[edge.EntityReference]; !exists {
				referencedEntity := domain.SyncEntityDTO{
					Reference: edge.EntityReference,
					Type:      edge.EntityType,
				}
				entitiesMap[edge.EntityReference] = referencedEntity
				c.logger.Debug("Created entity for edge reference",
					"reference", edge.EntityReference,
					"type", edge.EntityType,
					"referencedBy", kafkaEntityMessage.Reference)
			}

			// Create relationship
			relationship := domain.SyncRelationshipDTO{
				SourceReference:  kafkaEntityMessage.Reference,
				TargetReference:  edge.EntityReference,
				RelationshipType: edge.RelationType,
			}
			relationships = append(relationships, relationship)
		}
	}

	// Convert entities map to slice
	entities := make([]domain.SyncEntityDTO, 0, len(entitiesMap))
	for _, entity := range entitiesMap {
		entities = append(entities, entity)
	}

	// Create final sync request
	syncRequest := domain.SyncGraphRequest{
		Entities:      entities,
		Relationships: relationships,
	}

	// Call GraphWriteRepository.SyncGraph
	if err := c.graphWriteRepository.SyncGraph(ctx, syncRequest); err != nil {
		c.logger.Error("Failed to sync graph",
			"error", err,
			"entitiesCount", len(syncRequest.Entities),
			"relationshipsCount", len(syncRequest.Relationships))
		return fmt.Errorf("failed to sync graph: %w", err)
	}

	c.logger.Info("Successfully processed messages batch",
		"count", len(messages),
		"entitiesCount", len(syncRequest.Entities),
		"relationshipsCount", len(syncRequest.Relationships))

	return nil
}
