package entities

import (
	"encoding/json"
	"time"
)

type Granularity string

const (
	GranularityInstant Granularity = "instant"
	GranularityDay     Granularity = "day"
	GranularityWeek    Granularity = "week"
	GranularityMonth   Granularity = "month"
	GranularityQuarter Granularity = "quarter"
	GranularityYear    Granularity = "year"
)

type TemporalProperty struct {
	EntityID int64  `json:"entity_id"`
	Key      string `json:"key"`
	// O valor da propriedade, que pode ser qualquer estrutura JSON.
	Value          json.RawMessage `json:"value"`
	IdempotencyKey string          `json:"idempotency_key"`
	Granularity    Granularity     `json:"granularity"`
	ReferenceDate  time.Time       `json:"reference_date"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}
