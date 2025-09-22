package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"userprofilepoc/src/domain"
	"userprofilepoc/src/infra/kafka"
)

type DomainEventPublisher struct {
	logger      *slog.Logger
	kafkaClient *kafka.KafkaClient
	topic       string
}

func NewDomainEventPublisher(
	logger *slog.Logger,
	kafkaClient *kafka.KafkaClient,
	topic string,
) *DomainEventPublisher {
	return &DomainEventPublisher{
		logger:      logger,
		kafkaClient: kafkaClient,
		topic:       topic,
	}
}

// DomainEventWithMetadata wraps a domain event with metadata needed for headers
type DomainEventWithMetadata struct {
	domain.DomainEvent
	EventID   string
	EventType string
}

// PublishDomainEvents publishes a batch of domain events to Kafka
func (p *DomainEventPublisher) PublishDomainEvents(ctx context.Context, events []DomainEventWithMetadata) error {
	if len(events) == 0 {
		return nil
	}

	p.logger.Debug("Publishing domain events batch", "count", len(events))

	kafkaMessages := make([]kafka.Message, 0, len(events))

	for _, eventWithMetadata := range events {
		// Serialize only the domain event (without metadata)
		eventBytes, err := json.Marshal(eventWithMetadata.DomainEvent)
		if err != nil {
			p.logger.Error("Failed to marshal domain event",
				"error", err,
				"event_id", eventWithMetadata.EventID,
				"entity_reference", eventWithMetadata.Data.Reference)
			continue
		}

		// Create Kafka message with headers for filtering
		headers := p.createEventHeaders(eventWithMetadata)
		kafkaMsg := kafka.Message{
			Key:     eventWithMetadata.Data.Reference, // Partition by entity for ordering
			Value:   eventBytes,
			Headers: headers,
		}

		kafkaMessages = append(kafkaMessages, kafkaMsg)

		p.logger.Debug("Prepared domain event for publishing",
			"event_id", eventWithMetadata.EventID,
			"entity_reference", eventWithMetadata.Data.Reference,
			"event_type", eventWithMetadata.EventType)
	}

	// Publish to Kafka
	if err := p.kafkaClient.Producer(kafkaMessages, p.topic); err != nil {
		p.logger.Error("Failed to publish domain events to Kafka",
			"error", err,
			"topic", p.topic,
			"events_count", len(kafkaMessages))
		return fmt.Errorf("failed to publish domain events to topic %s: %w", p.topic, err)
	}

	p.logger.Info("Successfully published domain events",
		"topic", p.topic,
		"events_count", len(kafkaMessages))

	return nil
}

// createEventHeaders creates Kafka headers for event filtering (SNS-like)
func (p *DomainEventPublisher) createEventHeaders(eventWithMetadata DomainEventWithMetadata) map[string]string {
	headers := map[string]string{
		"event_type":     eventWithMetadata.EventType,
		"source_service": "user-profile-api",
		"schema_version": "v1",
		"event_id":       eventWithMetadata.EventID,
	}

	// Add entity_type header only if Type field is present
	if eventWithMetadata.Data.Type != "" {
		headers["entity_type"] = eventWithMetadata.Data.Type
	}

	// Add fields_changed header for all events
	fieldsChanged := p.extractChangedFieldsFromProperties(eventWithMetadata.DomainEvent, eventWithMetadata.EventType)
	if len(fieldsChanged) > 0 {
		headers["fields_changed"] = strings.Join(fieldsChanged, ",")
	}

	// Add relation_type header for relationship events
	if strings.Contains(eventWithMetadata.EventType, "relationship") {
		if relationType, exists := eventWithMetadata.Data.Properties["relationship_type"]; exists {
			if newVal := relationType.New; newVal != nil {
				headers["relation_type"] = fmt.Sprintf("%v", newVal)
			}
		}
	}

	// Add property_type and granularity headers for temporal events
	if strings.Contains(eventWithMetadata.EventType, "temporal") {
		if key, exists := eventWithMetadata.Data.Properties["key"]; exists {
			if newVal := key.New; newVal != nil {
				headers["property_type"] = fmt.Sprintf("%v", newVal)
			}
		}
		if granularity, exists := eventWithMetadata.Data.Properties["granularity"]; exists {
			if newVal := granularity.New; newVal != nil {
				headers["granularity"] = fmt.Sprintf("%v", newVal)
			}
		}
	}

	return headers
}

// extractChangedFieldsFromProperties extracts field names from the properties structure
func (p *DomainEventPublisher) extractChangedFieldsFromProperties(event domain.DomainEvent, eventType string) []string {
	var fields []string

	// For temporal events, extract fields from the value JSON
	if strings.Contains(eventType, "temporal") {
		if valueProperty, exists := event.Data.Properties["value"]; exists {
			if newVal := valueProperty.New; newVal != nil {
				// The value is already a JSON string, parse it directly
				var valueStr string
				if str, ok := newVal.(string); ok {
					valueStr = str
				} else {
					// If not string, marshal it first
					if valueBytes, err := json.Marshal(newVal); err == nil {
						valueStr = string(valueBytes)
					}
				}

				if valueStr != "" {
					var valueMap map[string]interface{}
					if err := json.Unmarshal([]byte(valueStr), &valueMap); err == nil {
						for field := range valueMap {
							fields = append(fields, field)
						}
					}
				}
			}
		}
		// Also include other temporal properties like key, granularity, reference_date
		for field := range event.Data.Properties {
			if field != "value" {
				fields = append(fields, field)
			}
		}
	} else {
		// For entity and relationship events, use property names directly
		for field := range event.Data.Properties {
			fields = append(fields, field)
		}
	}

	return fields
}

// PublishSingleEvent is a convenience method to publish a single domain event
func (p *DomainEventPublisher) PublishSingleEvent(ctx context.Context, event DomainEventWithMetadata) error {
	return p.PublishDomainEvents(ctx, []DomainEventWithMetadata{event})
}
