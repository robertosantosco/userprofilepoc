package stubs

import (
	"encoding/json"
	"time"
	"userprofilepoc/src/domain/entities"

	"github.com/brianvoe/gofakeit/v6"
)

type TemporalPropertyStub struct {
	property entities.TemporalProperty
}

func NewTemporalPropertyStub() TemporalPropertyStub {
	now := time.Now().UTC()

	value := map[string]interface{}{
		"amount":   gofakeit.Float64Range(100, 10000),
		"currency": "BRL",
	}
	valueJSON, _ := json.Marshal(value)

	property := entities.TemporalProperty{
		EntityID:       gofakeit.Int64(),
		Key:            "revenue",
		Value:          valueJSON,
		IdempotencyKey: gofakeit.UUID(),
		Granularity:    entities.GranularityMonth,
		ReferenceDate:  now.AddDate(0, -1, 0), // Last month
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	return TemporalPropertyStub{property: property}
}

func (tps TemporalPropertyStub) WithEntityID(entityID int64) TemporalPropertyStub {
	tps.property.EntityID = entityID
	return tps
}

func (tps TemporalPropertyStub) WithKey(key string) TemporalPropertyStub {
	tps.property.Key = key
	return tps
}

func (tps TemporalPropertyStub) WithValue(value interface{}) TemporalPropertyStub {
	valueJSON, _ := json.Marshal(value)
	tps.property.Value = valueJSON
	return tps
}

func (tps TemporalPropertyStub) WithIdempotencyKey(key string) TemporalPropertyStub {
	tps.property.IdempotencyKey = key
	return tps
}

func (tps TemporalPropertyStub) WithGranularity(granularity entities.Granularity) TemporalPropertyStub {
	tps.property.Granularity = granularity
	return tps
}

func (tps TemporalPropertyStub) WithReferenceDate(referenceDate time.Time) TemporalPropertyStub {
	tps.property.ReferenceDate = referenceDate
	return tps
}

func (tps TemporalPropertyStub) Get() entities.TemporalProperty {
	return tps.property
}
