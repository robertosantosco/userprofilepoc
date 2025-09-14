package entities

import (
	"encoding/json"
	"time"
)

// É a "aresta" ou o relacionamento entre dois nós.
type Edge struct {
	ID               int64  `json:"id"`
	LeftEntityID     int64  `json:"left_entity_id"`
	RightEntityID    int64  `json:"right_entity_id"`
	RelationshipType string `json:"relationship_type"`
	// Metadados sobre o próprio relacionamento (ex: papel do usuário em uma organização).
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}
