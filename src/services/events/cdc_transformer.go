package events

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"userprofilepoc/src/domain"
	"userprofilepoc/src/infra/debezium"

	"github.com/google/uuid"
)

type CDCTransformer struct {
	logger *slog.Logger
}

func NewCDCTransformer(logger *slog.Logger) *CDCTransformer {
	return &CDCTransformer{
		logger: logger,
	}
}

// TransformCDCEvent converts a CDC event to one or more domain events
func (t *CDCTransformer) TransformCDCEvent(ctx context.Context, cdcEvent *debezium.CDCEvent) ([]DomainEventWithMetadata, error) {
	tableName := cdcEvent.Source.Table

	t.logger.Debug("Processing CDC event",
		"table", tableName,
		"operation", cdcEvent.Operation,
		"ts_ms", cdcEvent.TsMs)

	switch tableName {
	case domain.TableEntities:
		return t.transformEntityEvent(cdcEvent)
	case domain.TableEdges:
		return t.transformEdgeEvent(cdcEvent)
	case domain.TableTemporalProperties:
		return t.transformTemporalEvent(cdcEvent)
	default:
		// Handle partitioned temporal_properties tables
		if strings.HasPrefix(tableName, "temporal_properties") {
			t.logger.Debug("Processing partitioned temporal table", "table", tableName)
			return t.transformTemporalEvent(cdcEvent)
		}

		t.logger.Debug("Ignoring CDC event from unknown table", "table", tableName)
		return nil, nil
	}
}

// transformEntityEvent converts entity table CDC events to domain events
func (t *CDCTransformer) transformEntityEvent(cdcEvent *debezium.CDCEvent) ([]DomainEventWithMetadata, error) {
	var events []DomainEventWithMetadata

	operation := t.mapCDCOperation(cdcEvent.Operation)
	eventType := t.mapToEventType(operation, cdcEvent.Source.Table)
	eventTimestamp := time.UnixMilli(cdcEvent.TsMs).UTC()

	var entityRef, entityType string
	var oldProperties, newProperties map[string]interface{}

	// Extract entity reference and type
	if cdcEvent.After != nil {
		entityRef, _ = cdcEvent.After["reference"].(string)
		entityType, _ = cdcEvent.After["type"].(string)
		if props := cdcEvent.After["properties"]; props != nil {
			if propsStr, ok := props.(string); ok {
				json.Unmarshal([]byte(propsStr), &newProperties)
			} else {
				newProperties = props.(map[string]interface{})
			}
		}
	}

	if cdcEvent.Before != nil {
		if entityRef == "" {
			entityRef, _ = cdcEvent.Before["reference"].(string)
		}
		if entityType == "" {
			entityType, _ = cdcEvent.Before["type"].(string)
		}
		if props := cdcEvent.Before["properties"]; props != nil {
			if propsStr, ok := props.(string); ok {
				json.Unmarshal([]byte(propsStr), &oldProperties)
			} else {
				oldProperties = props.(map[string]interface{})
			}
		}
	}

	if entityRef == "" || entityType == "" {
		return nil, fmt.Errorf("missing entity reference or type in CDC event")
	}

	// Create properties in new format
	properties := make(map[string]domain.PropertyPair)

	switch operation {
	case domain.OperationInsert:
		// All properties are new
		for field, value := range newProperties {
			properties[field] = domain.PropertyPair{
				Old: nil,
				New: value,
			}
		}

	case domain.OperationUpdate:
		// Handle case where before data is missing
		if len(oldProperties) == 0 {
			t.logger.Warn("UPDATE event missing 'before' data, treating as insert-like",
				"entity_ref", entityRef,
				"entity_type", entityType)
			// Treat as insert-like when no before data
			for field, value := range newProperties {
				properties[field] = domain.PropertyPair{
					Old: nil,
					New: value,
				}
			}
		} else {
			// Normal update with before/after comparison
			allFields := make(map[string]bool)
			for field := range oldProperties {
				allFields[field] = true
			}
			for field := range newProperties {
				allFields[field] = true
			}

			for field := range allFields {
				oldVal := oldProperties[field]
				newVal := newProperties[field]

				// Only include if values are different
				if !t.compareValues(oldVal, newVal) {
					properties[field] = domain.PropertyPair{
						Old: oldVal,
						New: newVal,
					}
				}
			}
		}

	case domain.OperationDelete:
		// All properties are removed
		for field, value := range oldProperties {
			properties[field] = domain.PropertyPair{
				Old: value,
				New: nil,
			}
		}
	}

	// Create domain event with metadata
	eventID := uuid.New().String()
	event := DomainEventWithMetadata{
		DomainEvent: domain.DomainEvent{
			IdempotencyKey: t.generateIdempotencyKey("entity", entityRef, entityType, eventTimestamp),
			EventTimestamp: eventTimestamp,
			Data: domain.DomainEventData{
				Type:       entityType,
				Reference:  entityRef,
				Properties: properties,
			},
		},
		EventID:   eventID,
		EventType: eventType,
	}

	events = append(events, event)

	t.logger.Debug("Transformed entity CDC event",
		"entity_reference", entityRef,
		"entity_type", entityType,
		"event_type", eventType,
		"properties_changed", len(properties))

	return events, nil
}

// transformEdgeEvent converts edge table CDC events to domain events
func (t *CDCTransformer) transformEdgeEvent(cdcEvent *debezium.CDCEvent) ([]DomainEventWithMetadata, error) {
	var events []DomainEventWithMetadata

	operation := t.mapCDCOperation(cdcEvent.Operation)
	eventType := t.mapToEventType(operation, cdcEvent.Source.Table)
	eventTimestamp := time.UnixMilli(cdcEvent.TsMs).UTC()

	var leftEntityID, rightEntityID int64
	var oldRelationshipType, newRelationshipType string
	var oldMetadata, newMetadata interface{}

	// Extract relationship data
	if cdcEvent.After != nil {
		if val, ok := cdcEvent.After["left_entity_id"].(float64); ok {
			leftEntityID = int64(val)
		}
		if val, ok := cdcEvent.After["right_entity_id"].(float64); ok {
			rightEntityID = int64(val)
		}
		newRelationshipType, _ = cdcEvent.After["relationship_type"].(string)
		newMetadata = cdcEvent.After["metadata"]
	}

	if cdcEvent.Before != nil {
		if leftEntityID == 0 {
			if val, ok := cdcEvent.Before["left_entity_id"].(float64); ok {
				leftEntityID = int64(val)
			}
		}
		if rightEntityID == 0 {
			if val, ok := cdcEvent.Before["right_entity_id"].(float64); ok {
				rightEntityID = int64(val)
			}
		}
		oldRelationshipType, _ = cdcEvent.Before["relationship_type"].(string)
		oldMetadata = cdcEvent.Before["metadata"]
	}

	// Create properties for relationship
	properties := make(map[string]domain.PropertyPair)

	switch operation {
	case domain.OperationInsert:
		properties["relationship_type"] = domain.PropertyPair{
			Old: nil,
			New: newRelationshipType,
		}
		if newMetadata != nil {
			properties["metadata"] = domain.PropertyPair{
				Old: nil,
				New: newMetadata,
			}
		}

	case domain.OperationUpdate:
		// Handle missing before data
		if cdcEvent.Before == nil {
			t.logger.Warn("UPDATE relationship event missing 'before' data, treating as insert-like",
				"left_entity_id", leftEntityID,
				"right_entity_id", rightEntityID)
			properties["relationship_type"] = domain.PropertyPair{
				Old: nil,
				New: newRelationshipType,
			}
			if newMetadata != nil {
				properties["metadata"] = domain.PropertyPair{
					Old: nil,
					New: newMetadata,
				}
			}
		} else {
			// Normal comparison
			if oldRelationshipType != newRelationshipType {
				properties["relationship_type"] = domain.PropertyPair{
					Old: oldRelationshipType,
					New: newRelationshipType,
				}
			}
			if !t.compareValues(oldMetadata, newMetadata) {
				properties["metadata"] = domain.PropertyPair{
					Old: oldMetadata,
					New: newMetadata,
				}
			}
		}

	case domain.OperationDelete:
		properties["relationship_type"] = domain.PropertyPair{
			Old: oldRelationshipType,
			New: nil,
		}
		if oldMetadata != nil {
			properties["metadata"] = domain.PropertyPair{
				Old: oldMetadata,
				New: nil,
			}
		}
	}

	targetEntityReference := fmt.Sprintf("entity-%d", rightEntityID)

	// Create domain event for source entity with metadata
	eventID := uuid.New().String()
	event := DomainEventWithMetadata{
		DomainEvent: domain.DomainEvent{
			IdempotencyKey: t.generateIdempotencyKey("relationship", fmt.Sprintf("entity-%d", leftEntityID), "unknown", eventTimestamp),
			EventTimestamp: eventTimestamp,
			Data: domain.DomainEventData{
				Reference:             fmt.Sprintf("entity-%d", leftEntityID),
				TargetEntityReference: &targetEntityReference,
				Properties:            properties,
			},
		},
		EventID:   eventID,
		EventType: eventType,
	}

	events = append(events, event)

	t.logger.Debug("Transformed edge CDC event",
		"left_entity_id", leftEntityID,
		"right_entity_id", rightEntityID,
		"event_type", eventType,
		"properties_changed", len(properties))

	return events, nil
}

// transformTemporalEvent converts temporal_properties CDC events to domain events
func (t *CDCTransformer) transformTemporalEvent(cdcEvent *debezium.CDCEvent) ([]DomainEventWithMetadata, error) {
	var events []DomainEventWithMetadata

	operation := t.mapCDCOperation(cdcEvent.Operation)
	eventTimestamp := time.UnixMilli(cdcEvent.TsMs).UTC()

	// Extract temporal data from CDC event - get both old and new values for all fields
	var entityRef, entityType string
	var newKey, newGranularity, newIdempotencyKey string
	var oldKey, oldGranularity, oldIdempotencyKey string
	var newReferenceDate, oldReferenceDate time.Time
	var oldValue, newValue interface{}

	// Extract NEW values from after (for create/update)
	if cdcEvent.After != nil {
		newKey, _ = cdcEvent.After["key"].(string)
		newGranularity, _ = cdcEvent.After["granularity"].(string)
		newIdempotencyKey, _ = cdcEvent.After["idempotency_key"].(string)

		if refDateStr, ok := cdcEvent.After["reference_date"].(string); ok {
			newReferenceDate, _ = time.Parse(time.RFC3339, refDateStr)
		}

		newValue = cdcEvent.After["value"]
		entityRef, entityType = t.extractEntityFromIdempotencyKey(newIdempotencyKey)
	}

	// Extract OLD values from before (for update/delete)
	if cdcEvent.Before != nil {
		oldKey, _ = cdcEvent.Before["key"].(string)
		oldGranularity, _ = cdcEvent.Before["granularity"].(string)
		oldIdempotencyKey, _ = cdcEvent.Before["idempotency_key"].(string)

		if refDateStr, ok := cdcEvent.Before["reference_date"].(string); ok {
			oldReferenceDate, _ = time.Parse(time.RFC3339, refDateStr)
		}

		oldValue = cdcEvent.Before["value"]

		// If we don't have new values, use old values for entity extraction
		if entityRef == "" && oldIdempotencyKey != "" {
			entityRef, entityType = t.extractEntityFromIdempotencyKey(oldIdempotencyKey)
		}
	}

	// Create properties in new format - always include context for temporal events
	properties := make(map[string]domain.PropertyPair)

	// Always include metadata for temporal events so consumers can understand the context
	// Use actual old/new values from CDC event for maximum flexibility
	properties["key"] = domain.PropertyPair{
		Old: t.nilIfEmpty(oldKey),
		New: t.nilIfEmpty(newKey),
	}
	properties["granularity"] = domain.PropertyPair{
		Old: t.nilIfEmpty(oldGranularity),
		New: t.nilIfEmpty(newGranularity),
	}
	properties["reference_date"] = domain.PropertyPair{
		Old: t.nilIfZeroTime(oldReferenceDate),
		New: t.nilIfZeroTime(newReferenceDate),
	}
	properties["idempotency_key"] = domain.PropertyPair{
		Old: t.nilIfEmpty(oldIdempotencyKey),
		New: t.nilIfEmpty(newIdempotencyKey),
	}

	// Add the main value property
	switch operation {
	case domain.OperationInsert:
		properties["value"] = domain.PropertyPair{
			Old: nil,
			New: newValue,
		}
	case domain.OperationUpdate:
		// Handle missing before data for temporal events
		if cdcEvent.Before == nil {
			t.logger.Warn("UPDATE temporal event missing 'before' data, treating as insert-like",
				"entity_ref", entityRef,
				"key", newKey)
			properties["value"] = domain.PropertyPair{
				Old: nil,
				New: newValue,
			}
		} else {
			if !t.compareValues(oldValue, newValue) {
				properties["value"] = domain.PropertyPair{
					Old: oldValue,
					New: newValue,
				}
			}
		}
	case domain.OperationDelete:
		properties["value"] = domain.PropertyPair{
			Old: oldValue,
			New: nil,
		}
	}

	// Determine the correct event type based on actual value changes
	var eventType string
	if oldValue == nil && newValue != nil {
		eventType = domain.EventTypeTemporalDataCreated
	} else if oldValue != nil && newValue == nil {
		eventType = domain.EventTypeTemporalDataDeleted
	} else if oldValue != nil && newValue != nil {
		eventType = domain.EventTypeTemporalDataUpdated
	} else {
		// Fallback to CDC operation mapping
		eventType = t.mapToEventType(operation, cdcEvent.Source.Table)
	}

	// Use the most current values for key generation
	currentKey := newKey
	currentReferenceDate := newReferenceDate
	if currentKey == "" {
		currentKey = oldKey
	}
	if currentReferenceDate.IsZero() {
		currentReferenceDate = oldReferenceDate
	}

	eventID := uuid.New().String()
	event := DomainEventWithMetadata{
		DomainEvent: domain.DomainEvent{
			IdempotencyKey: t.generateIdempotencyKey("temporal", entityRef, currentKey, currentReferenceDate),
			EventTimestamp: eventTimestamp,
			Data: domain.DomainEventData{
				// Remove Type field for temporal events to make them cleaner
				Reference:  entityRef,
				Properties: properties,
			},
		},
		EventID:   eventID,
		EventType: eventType,
	}

	events = append(events, event)

	t.logger.Debug("Transformed temporal CDC event",
		"entity_reference", entityRef,
		"entity_type", entityType,
		"event_type", eventType,
		"property_type", currentKey,
		"granularity", newGranularity)

	return events, nil
}

// Helper methods

func (t *CDCTransformer) mapCDCOperation(cdcOp string) string {
	// Use the mapping from debezium package for consistency
	return debezium.MapCDCOperation(cdcOp)
}

// mapToEventType converts CDC operation + table to domain event type
func (t *CDCTransformer) mapToEventType(operation, tableName string) string {
	switch {
	case tableName == domain.TableEntities:
		switch operation {
		case domain.OperationInsert:
			return domain.EventTypeEntityCreated
		case domain.OperationUpdate:
			return domain.EventTypeEntityPropertiesUpdated
		case domain.OperationDelete:
			return domain.EventTypeEntityDeleted
		}
	case tableName == domain.TableEdges:
		switch operation {
		case domain.OperationInsert:
			return domain.EventTypeRelationshipCreated
		case domain.OperationUpdate:
			return domain.EventTypeRelationshipUpdated
		case domain.OperationDelete:
			return domain.EventTypeRelationshipDeleted
		}
	case strings.HasPrefix(tableName, domain.TableTemporalProperties):
		switch operation {
		case domain.OperationInsert:
			return domain.EventTypeTemporalDataCreated
		case domain.OperationUpdate:
			return domain.EventTypeTemporalDataUpdated
		case domain.OperationDelete:
			return domain.EventTypeTemporalDataDeleted
		}
	}
	return "unknown_event_type"
}

func (t *CDCTransformer) findChangedFields(oldProps, newProps map[string]interface{}) []string {
	var changed []string

	// Check for changed and new fields
	for field, newVal := range newProps {
		if oldVal, exists := oldProps[field]; !exists || !t.compareValues(oldVal, newVal) {
			changed = append(changed, field)
		}
	}

	// Check for removed fields
	for field := range oldProps {
		if _, exists := newProps[field]; !exists {
			changed = append(changed, field)
		}
	}

	return changed
}

func (t *CDCTransformer) compareValues(a, b interface{}) bool {
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return string(aJSON) == string(bJSON)
}

func (t *CDCTransformer) generateIdempotencyKey(prefix, entityRef, entityType string, timestamp time.Time) string {
	baseKey := fmt.Sprintf("%s-%s:%s-%d", prefix, entityRef, entityType, timestamp.Unix())
	hash := md5.Sum([]byte(baseKey))
	return hex.EncodeToString(hash[:])
}

func (t *CDCTransformer) extractEntityFromIdempotencyKey(idempotencyKey string) (entityRef, entityType string) {
	// Parse idempotency key format: entityId:key:granularity:timestamp
	// This is a simplification - in real implementation would need entity lookup
	parts := strings.Split(idempotencyKey, ":")
	if len(parts) >= 1 {
		entityRef = fmt.Sprintf("entity-%s", parts[0])
		entityType = "unknown" // Would need database lookup
	}
	return
}

// nilIfEmpty returns nil if string is empty, otherwise returns the string
func (t *CDCTransformer) nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// nilIfZeroTime returns nil if time is zero, otherwise returns the time
func (t *CDCTransformer) nilIfZeroTime(timeVal time.Time) interface{} {
	if timeVal.IsZero() {
		return nil
	}
	return timeVal
}
