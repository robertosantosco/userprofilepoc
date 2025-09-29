package stubs

import (
	"encoding/json"
	"time"
	"userprofilepoc/src/domain/entities"

	"github.com/brianvoe/gofakeit/v6"
)

type EntityStub struct {
	entity entities.Entity
}

func NewEntityStub() EntityStub {
	now := time.Now().UTC()

	properties := map[string]interface{}{
		"name":  gofakeit.Name(),
		"email": gofakeit.Email(),
	}
	propsJSON, _ := json.Marshal(properties)

	entity := entities.Entity{
		ID:         gofakeit.Int64(),
		Type:       "user",
		Reference:  gofakeit.UUID(),
		Properties: propsJSON,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	return EntityStub{entity: entity}
}

func (es EntityStub) WithType(entityType string) EntityStub {
	es.entity.Type = entityType
	return es
}

func (es EntityStub) WithReference(reference string) EntityStub {
	es.entity.Reference = reference
	return es
}

func (es EntityStub) WithProperties(properties map[string]interface{}) EntityStub {
	propsJSON, _ := json.Marshal(properties)
	es.entity.Properties = propsJSON
	return es
}

func (es EntityStub) Get() entities.Entity {
	return es.entity
}
