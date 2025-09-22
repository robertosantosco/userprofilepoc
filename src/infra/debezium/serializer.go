package debezium

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CDCSerializer handles parsing and validation of CDC messages
type CDCSerializer struct {
	IncludeTables []string
}

// IsTableMonitored checks if table should be processed
func (s *CDCSerializer) IsTableMonitored(tableName string) bool {
	// If include list exists, only monitor included tables
	for _, included := range s.IncludeTables {
		// Exact match
		if tableName == included {
			return true
		}
		// Pattern match for partitioned tables (e.g., temporal_properties matches temporal_properties_p20250901)
		if strings.HasSuffix(included, "*") {
			prefix := strings.TrimSuffix(included, "*")
			if strings.HasPrefix(tableName, prefix) {
				return true
			}
		}
	}
	return false
}

// ParseCDCEvent deserializes Kafka message to CDC event
func (s *CDCSerializer) ParseCDCEvent(messageValue []byte) (*CDCEvent, error) {
	var cdcEvent CDCEvent
	if err := json.Unmarshal(messageValue, &cdcEvent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal CDC event: %w", err)
	}

	// Validate CDC event structure
	if err := s.validateCDCEvent(&cdcEvent); err != nil {
		return nil, fmt.Errorf("invalid CDC event: %w", err)
	}

	return &cdcEvent, nil
}

// validateCDCEvent performs basic validation on CDC event
func (s *CDCSerializer) validateCDCEvent(event *CDCEvent) error {
	if event.Source.Table == "" {
		return fmt.Errorf("missing source table")
	}

	if event.Operation == "" {
		return fmt.Errorf("missing operation")
	}

	// Validate operation type
	validOps := map[string]bool{"c": true, "u": true, "d": true, "r": true}
	if !validOps[event.Operation] {
		return fmt.Errorf("invalid operation: %s", event.Operation)
	}

	// For delete operations, 'before' should be present
	if event.Operation == "d" && event.Before == nil {
		return fmt.Errorf("missing 'before' data for delete operation")
	}

	// For update operations, warn if 'before' is missing but don't fail
	if event.Operation == "u" && event.Before == nil {
		// This can happen with some Debezium configurations
		// We'll process it as an insert-like operation
	}

	// For create/update operations, 'after' should be present
	if (event.Operation == "c" || event.Operation == "u") && event.After == nil {
		return fmt.Errorf("missing 'after' data for operation %s", event.Operation)
	}

	return nil
}

// ShouldProcessEvent checks if CDC event should be processed based on filtering rules
func (s *CDCSerializer) ShouldProcessEvent(event *CDCEvent) bool {
	tableName := event.Source.Table

	// Check table filter
	if !s.IsTableMonitored(tableName) {
		return false
	}

	// Skip snapshot operations if configured (optional)
	if event.Source.Snapshot == "true" {
		// For POC, we might want to process snapshots
		// In production, you might want to skip them
		return true
	}

	return true
}

// MapCDCOperation converts CDC operation code to domain operation
func MapCDCOperation(cdcOp string) string {
	switch cdcOp {
	case "c":
		return "INSERT"
	case "u":
		return "UPDATE"
	case "d":
		return "DELETE"
	case "r":
		return "INSERT" // Read (snapshot) treated as insert
	default:
		return "UNKNOWN"
	}
}
