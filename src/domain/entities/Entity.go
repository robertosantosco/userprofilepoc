package entities

import (
	"encoding/json"
	"time"
)

// É o "nó" do nosso grafo.
type Entity struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Reference string `json:"reference"`
	// Usamos json.RawMessage para dados estáticos, pois permite
	// manter o JSON original sem precisar de uma struct específica,
	// aumentando a flexibilidade.
	Properties json.RawMessage `json:"properties,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}
