package consumers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
	"userprofilepoc/src/domain"
	"userprofilepoc/src/infra/kafka"
	"userprofilepoc/src/repositories"
)

// PropertyMessage representa o schema da mensagem Kafka para propriedades
type PropertyMessage struct {
	EntityReference string `json:"entity_reference"`
	EntityType      string `json:"entity_type"`
	FieldName       string `json:"field_name"`
	FieldValue      string `json:"field_value"`
	ReferenceDate   string `json:"reference_date"`
}

// PropertyValue armazena valor e timestamp para resolução de conflitos (internal)
type PropertyValue struct {
	Value         string    `json:"value"`
	ReferenceDate time.Time `json:"reference_date"`
}

type EntityPropertiesConsumer struct {
	logger               *slog.Logger
	graphWriteRepository *repositories.GraphWriteRepository
}

func NewEntityPropertiesConsumer(
	logger *slog.Logger,
	graphWriteRepository *repositories.GraphWriteRepository,
) *EntityPropertiesConsumer {
	return &EntityPropertiesConsumer{
		logger:               logger,
		graphWriteRepository: graphWriteRepository,
	}
}

func (c *EntityPropertiesConsumer) Start(ctx context.Context, kafkaClient *kafka.KafkaClient, topic string) error {
	c.logger.Info("Starting entity properties consumer", "topic", topic)

	handler := func(messages []kafka.Message) error {
		return c.handleMessages(ctx, messages)
	}

	return kafkaClient.Consumer(ctx, handler, topic)
}

func (c *EntityPropertiesConsumer) handleMessages(ctx context.Context, messages []kafka.Message) error {
	if len(messages) == 0 {
		return nil
	}

	c.logger.Info("Processing properties messages batch", "count", len(messages))

	// Track entities and their properties with conflict resolution
	entitiesProperties := make(map[string]map[string]PropertyValue) // entityKey -> fieldName -> PropertyValue

	for _, msg := range messages {
		var propertyMsg PropertyMessage
		if err := json.Unmarshal(msg.Value, &propertyMsg); err != nil {
			c.logger.Error("Failed to unmarshal property message",
				"error", err,
				"key", msg.Key,
				"value", string(msg.Value))
			return fmt.Errorf("failed to unmarshal property message with key %s: %w", msg.Key, err)
		}

		// Validate required fields
		if propertyMsg.EntityReference == "" || propertyMsg.EntityType == "" ||
			propertyMsg.FieldName == "" || propertyMsg.FieldValue == "" {
			c.logger.Error("Invalid property message: missing required fields",
				"key", msg.Key,
				"entity_reference", propertyMsg.EntityReference,
				"entity_type", propertyMsg.EntityType,
				"field_name", propertyMsg.FieldName,
				"field_value", propertyMsg.FieldValue)
			return fmt.Errorf("invalid property message with key %s: missing required fields", msg.Key)
		}

		// Parse reference date
		referenceDate, err := c.parseReferenceDate(propertyMsg.ReferenceDate)
		if err != nil {
			c.logger.Error("Failed to parse reference_date",
				"error", err,
				"key", msg.Key,
				"reference_date", propertyMsg.ReferenceDate)
			return fmt.Errorf("failed to parse reference_date for message with key %s: %w", msg.Key, err)
		}

		// Create entity key
		entityKey := fmt.Sprintf("%s|%s", propertyMsg.EntityReference, propertyMsg.EntityType)

		// Initialize entity properties map if not exists
		if _, exists := entitiesProperties[entityKey]; !exists {
			entitiesProperties[entityKey] = make(map[string]PropertyValue)
		}

		// Check if this property value is newer than existing one
		existingProp, hasExisting := entitiesProperties[entityKey][propertyMsg.FieldName]
		if !hasExisting || referenceDate.After(existingProp.ReferenceDate) {
			entitiesProperties[entityKey][propertyMsg.FieldName] = PropertyValue{
				Value:         propertyMsg.FieldValue,
				ReferenceDate: referenceDate,
			}

			if hasExisting {
				c.logger.Debug("Updated property with newer value",
					"entity_reference", propertyMsg.EntityReference,
					"field_name", propertyMsg.FieldName,
					"old_date", existingProp.ReferenceDate,
					"new_date", referenceDate)
			}
		} else {
			c.logger.Debug("Skipped older property value",
				"entity_reference", propertyMsg.EntityReference,
				"field_name", propertyMsg.FieldName,
				"existing_date", existingProp.ReferenceDate,
				"message_date", referenceDate)
		}
	}

	// Convert aggregated properties to SyncEntityDTO
	entities := make([]domain.SyncEntityDTO, 0, len(entitiesProperties))
	for entityKey, properties := range entitiesProperties {
		entityRef, entityType := c.parseEntityKey(entityKey)

		// Create final properties JSON with value and reference_date
		finalProperties := make(map[string]PropertyValue)
		for fieldName, propValue := range properties {
			finalProperties[fieldName] = propValue
		}

		propertiesJSON, err := json.Marshal(finalProperties)
		if err != nil {
			c.logger.Error("Failed to marshal properties",
				"error", err,
				"entity_reference", entityRef,
				"entity_type", entityType)
			return fmt.Errorf("failed to marshal properties for entity %s: %w", entityRef, err)
		}

		entities = append(entities, domain.SyncEntityDTO{
			Reference:  entityRef,
			Type:       entityType,
			Properties: json.RawMessage(propertiesJSON),
		})
	}

	// Create sync request (only entities, no relationships for properties)
	syncRequest := domain.SyncGraphRequest{
		Entities:      entities,
		Relationships: []domain.SyncRelationshipDTO{}, // Empty for properties
	}

	// Call GraphWriteRepository.SyncGraph
	if err := c.graphWriteRepository.SyncGraph(ctx, syncRequest); err != nil {
		c.logger.Error("Failed to sync entity properties",
			"error", err,
			"entitiesCount", len(syncRequest.Entities))
		return fmt.Errorf("failed to sync entity properties: %w", err)
	}

	c.logger.Info("Successfully processed properties batch",
		"count", len(messages),
		"entitiesAggregated", len(entities),
		"totalProperties", c.countTotalProperties(entitiesProperties))

	return nil
}

// parseReferenceDate parses the reference date string to time.Time
func (c *EntityPropertiesConsumer) parseReferenceDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Now().UTC(), nil // Default to current time if empty
	}

	// Try multiple common formats
	formats := []string{
		"2006-01-02 15:04:05",      // "2025-10-01 00:00:00"
		"2006-01-02T15:04:05Z",     // ISO format with Z
		"2006-01-02T15:04:05.000Z", // ISO format with milliseconds
		time.RFC3339,               // "2006-01-02T15:04:05Z07:00"
		"2006-01-02",               // Date only
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date %s with any known format", dateStr)
}

// parseEntityKey splits the entity key back to reference and type
func (c *EntityPropertiesConsumer) parseEntityKey(entityKey string) (string, string) {
	// Find the last occurrence of "|" to handle references that might contain "|"
	for i := len(entityKey) - 1; i >= 0; i-- {
		if entityKey[i] == '|' {
			return entityKey[:i], entityKey[i+1:]
		}
	}
	// Fallback if no "|" found (shouldn't happen with our logic)
	return entityKey, "unknown"
}

// countTotalProperties counts total properties across all entities
func (c *EntityPropertiesConsumer) countTotalProperties(entitiesProperties map[string]map[string]PropertyValue) int {
	total := 0
	for _, properties := range entitiesProperties {
		total += len(properties)
	}
	return total
}
