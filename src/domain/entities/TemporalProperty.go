package entities

import (
	"encoding/json"
	"time"
)

// Armazena um atributo de uma entidade que muda ao longo do tempo.
type TemporalProperty struct {
	EntityID int64  `json:"entity_id"`
	Key      string `json:"key"`
	// O valor da propriedade, que pode ser qualquer estrutura JSON.
	Value json.RawMessage `json:"value"`
	// O período de validade. Precisaria de um tipo customizado para escanear TSTZRANGE.
	// Para simplificar, podemos ler como string e fazer o parse na aplicação.
	PeriodStart time.Time  `json:"period_start"`
	PeriodEnd   *time.Time `json:"period_end"`
	Granularity string     `json:"granularity"`
	StartTS     time.Time  `json:"start_ts"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
