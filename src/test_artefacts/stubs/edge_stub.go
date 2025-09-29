package stubs

import (
	"encoding/json"
	"time"
	"userprofilepoc/src/domain/entities"

	"github.com/brianvoe/gofakeit/v6"
)

type EdgeStub struct {
	edge entities.Edge
}

func NewEdgeStub() EdgeStub {
	now := time.Now().UTC()

	metadata := map[string]interface{}{}
	metadataJSON, _ := json.Marshal(metadata)

	edge := entities.Edge{
		ID:               gofakeit.Int64(),
		LeftEntityID:     gofakeit.Int64(),
		RightEntityID:    gofakeit.Int64(),
		RelationshipType: "member_of",
		Metadata:         metadataJSON,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	return EdgeStub{edge: edge}
}

func (es EdgeStub) WithLeftEntityID(leftEntityID int64) EdgeStub {
	es.edge.LeftEntityID = leftEntityID
	return es
}

func (es EdgeStub) WithRightEntityID(rightEntityID int64) EdgeStub {
	es.edge.RightEntityID = rightEntityID
	return es
}

func (es EdgeStub) WithRelationshipType(relationshipType string) EdgeStub {
	es.edge.RelationshipType = relationshipType
	return es
}

func (es EdgeStub) WithMetadata(metadata map[string]interface{}) EdgeStub {
	metadataJSON, _ := json.Marshal(metadata)
	es.edge.Metadata = metadataJSON
	return es
}

func (es EdgeStub) Get() entities.Edge {
	return es.edge
}
