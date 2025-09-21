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

// TemporalMessage representa o schema da mensagem Kafka para dados temporais
type TemporalMessage struct {
	EntityReference       string      `json:"entity_reference"`
	EntityType            string      `json:"entity_type"`
	PropertyType          string      `json:"property_type"`
	PropertyValue         interface{} `json:"property_value"`
	PropertyReferenceDate string      `json:"property_reference_date"`
	PropertyGranularity   string      `json:"property_granularity"`
}

type TemporalDataConsumer struct {
	logger                  *slog.Logger
	temporalWriteRepository *repositories.TemporalWriteRepository
}

func NewTemporalDataConsumer(
	logger *slog.Logger,
	temporalWriteRepository *repositories.TemporalWriteRepository,
) *TemporalDataConsumer {
	return &TemporalDataConsumer{
		logger:                  logger,
		temporalWriteRepository: temporalWriteRepository,
	}
}

func (c *TemporalDataConsumer) Start(ctx context.Context, kafkaClient *kafka.KafkaClient, topic string) error {
	c.logger.Info("Starting temporal data consumer", "topic", topic)

	handler := func(messages []kafka.Message) error {
		return c.handleMessages(ctx, messages)
	}

	return kafkaClient.Consumer(ctx, handler, topic)
}

func (c *TemporalDataConsumer) handleMessages(ctx context.Context, messages []kafka.Message) error {
	if len(messages) == 0 {
		return nil
	}

	c.logger.Info("Processing temporal data messages batch", "count", len(messages))

	// Track temporal data points with deduplication by key (without reference_date)
	// Key: entity_reference|entity_type|property_type|granularity
	// We'll keep the one with the latest reference_date when there are conflicts
	temporalDataMap := make(map[string]domain.TemporalDataPointDTO)

	for _, msg := range messages {
		var temporalMsg TemporalMessage
		if err := json.Unmarshal(msg.Value, &temporalMsg); err != nil {
			c.logger.Error("Failed to unmarshal temporal message",
				"error", err,
				"key", msg.Key,
				"value", string(msg.Value))
			return fmt.Errorf("failed to unmarshal temporal message with key %s: %w", msg.Key, err)
		}

		// Validate required fields
		if temporalMsg.EntityReference == "" || temporalMsg.EntityType == "" ||
			temporalMsg.PropertyType == "" || temporalMsg.PropertyReferenceDate == "" ||
			temporalMsg.PropertyGranularity == "" {
			c.logger.Error("Invalid temporal message: missing required fields",
				"key", msg.Key,
				"entity_reference", temporalMsg.EntityReference,
				"entity_type", temporalMsg.EntityType,
				"property_type", temporalMsg.PropertyType,
				"reference_date", temporalMsg.PropertyReferenceDate,
				"granularity", temporalMsg.PropertyGranularity)
			return fmt.Errorf("invalid temporal message with key %s: missing required fields", msg.Key)
		}

		// Parse reference date
		referenceDate, err := c.parseReferenceDate(temporalMsg.PropertyReferenceDate)
		if err != nil {
			c.logger.Error("Failed to parse property_reference_date",
				"error", err,
				"key", msg.Key,
				"reference_date", temporalMsg.PropertyReferenceDate)
			return fmt.Errorf("failed to parse property_reference_date for message with key %s: %w", msg.Key, err)
		}

		// Convert property_value to JSON
		valueJSON, err := json.Marshal(temporalMsg.PropertyValue)
		if err != nil {
			c.logger.Error("Failed to marshal property_value",
				"error", err,
				"key", msg.Key,
				"property_value", temporalMsg.PropertyValue)
			return fmt.Errorf("failed to marshal property_value for message with key %s: %w", msg.Key, err)
		}

		// Create TemporalDataPointDTO
		dataPoint := domain.TemporalDataPointDTO{
			EntityReference: temporalMsg.EntityReference,
			EntityType:      temporalMsg.EntityType,
			Key:             temporalMsg.PropertyType,
			Value:           json.RawMessage(valueJSON),
			Granularity:     temporalMsg.PropertyGranularity,
			ReferenceDate:   referenceDate,
		}

		// Create unique key for deduplication (without reference_date to handle conflicts)
		dataPointKey := fmt.Sprintf("%s|%s|%s|%s",
			temporalMsg.EntityReference,
			temporalMsg.EntityType,
			temporalMsg.PropertyType,
			temporalMsg.PropertyGranularity)

		// Check if we already have this data point and keep the one with latest reference_date
		if existingDataPoint, exists := temporalDataMap[dataPointKey]; exists {
			if referenceDate.After(existingDataPoint.ReferenceDate) {
				// This message has newer reference_date, replace existing
				temporalDataMap[dataPointKey] = dataPoint
				c.logger.Debug("Updated temporal data point with newer reference_date",
					"entity_reference", temporalMsg.EntityReference,
					"property_type", temporalMsg.PropertyType,
					"old_date", existingDataPoint.ReferenceDate,
					"new_date", referenceDate)
			} else {
				// Existing has newer or same reference_date, skip this message
				c.logger.Debug("Skipped temporal data point with older reference_date",
					"entity_reference", temporalMsg.EntityReference,
					"property_type", temporalMsg.PropertyType,
					"existing_date", existingDataPoint.ReferenceDate,
					"message_date", referenceDate)
			}
		} else {
			// First occurrence of this data point
			temporalDataMap[dataPointKey] = dataPoint
			c.logger.Debug("Added temporal data point",
				"entity_reference", temporalMsg.EntityReference,
				"entity_type", temporalMsg.EntityType,
				"property_type", temporalMsg.PropertyType,
				"granularity", temporalMsg.PropertyGranularity,
				"reference_date", referenceDate)
		}
	}

	// Convert map to slice for sync request
	dataPoints := make([]domain.TemporalDataPointDTO, 0, len(temporalDataMap))
	for _, dataPoint := range temporalDataMap {
		dataPoints = append(dataPoints, dataPoint)
	}

	// Create sync request with deduplicated data points
	syncRequest := domain.SyncTemporalPropertyRequest{
		DataPoints: dataPoints,
	}

	// Call TemporalWriteRepository.UpsertDataPoints
	if err := c.temporalWriteRepository.UpsertDataPoints(ctx, syncRequest); err != nil {
		c.logger.Error("Failed to upsert temporal data points",
			"error", err,
			"dataPointsCount", len(syncRequest.DataPoints))
		return fmt.Errorf("failed to upsert temporal data points: %w", err)
	}

	c.logger.Info("Successfully processed temporal data batch",
		"count", len(messages),
		"dataPointsCreated", len(dataPoints),
		"duplicatesRemoved", len(messages)-len(dataPoints))

	return nil
}

// parseReferenceDate parses the reference date string to time.Time
func (c *TemporalDataConsumer) parseReferenceDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Now().UTC(), nil // Default to current time if empty
	}

	// Try multiple common formats
	formats := []string{
		time.RFC3339,               // "2006-01-02T15:04:05Z07:00" - ISO 8601
		"2006-01-02T15:04:05Z",     // ISO format with Z
		"2006-01-02T15:04:05.000Z", // ISO format with milliseconds
		"2006-01-02 15:04:05",      // "2025-10-01 00:00:00"
		"2006-01-02",               // Date only
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date %s with any known format", dateStr)
}
