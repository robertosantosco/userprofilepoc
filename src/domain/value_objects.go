package domain

import (
	"encoding/json"
	"errors"
	"time"
	"userprofilepoc/src/domain/entities"
)

var (
	ErrEntityNotFound = errors.New("entity not found")

	ErrUnavailableServer = errors.New("Oops, something unexpected happened. Please try again later.")
)

// ############################################################
// ############# PROCESSO DE LEITURA DO GRAFO #################
// ############################################################

// GraphNode representa um nó completo do grafo, contendo os dados
// da entidade e a informação estrutural de seus pais.
type GraphNode struct {
	entities.Entity
	ParentsInfo []ParentInfo
}

// ParentInfo descreve uma única relação de parentesco.
type ParentInfo struct {
	ParentID int64  `json:"parent_id"`
	Type     string `json:"type"`
}

type NodeTree struct {
	entities.Entity

	TemporalData []entities.TemporalProperty
	Edges        []*ProfileEdge
}

// A aresta agora aponta para outro NodeTree, criando a recursão.
type ProfileEdge struct {
	Type   string
	Entity *NodeTree
}

// ############################################################
// ######## PROCESSO DE ESCRITA DAS ENTITIES/EDGES ############
// ############################################################

// SyncEntityDTO define a estrutura de uma entidade para sincronização.
type SyncEntityDTO struct {
	Reference  string          `json:"reference"`
	Type       string          `json:"type"`
	Properties json.RawMessage `json:"properties"`
}

// SyncRelationshipDTO define a estrutura de um relacionamento para sincronização.
type SyncRelationshipDTO struct {
	SourceReference  string `json:"source_reference"`
	TargetReference  string `json:"target_reference"`
	RelationshipType string `json:"relationship_type"`
}

// SyncGraphRequest é o DTO completo que o serviço usa para solicitar uma sincronização.
type SyncGraphRequest struct {
	Entities      []SyncEntityDTO       `json:"entities"`
	Relationships []SyncRelationshipDTO `json:"relationships"`
}

// ############################################################
// ##### PROCESSO DE ESCRITA DAS TEMPORAL PROPERTIES ##########
// ############################################################

// TemporalDataPointDTO representa um único ponto de dado temporal a ser ingerido.
type TemporalDataPointDTO struct {
	EntityReference string          `json:"entity_reference"`
	EntityType      string          `json:"entity_type"`
	Key             string          `json:"key"`
	Value           json.RawMessage `json:"value"`
	Granularity     string          `json:"granularity"`
	ReferenceDate   time.Time       `json:"reference_date"`
}

type SyncTemporalPropertyRequest struct {
	DataPoints []TemporalDataPointDTO `json:"data_points"`
}

// ############################################################
// #####                DOMAIN EVENTS                ##########
// ############################################################

// DomainEvent represents a domain event emitted when data changes
type DomainEvent struct {
	IdempotencyKey string          `json:"event_idempotency_key"`
	EventTimestamp time.Time       `json:"event_timestamp"`
	Data           DomainEventData `json:"data"`
}

// DomainEventData contains the actual event data in consistent format
type DomainEventData struct {
	Type                  string                  `json:"type,omitempty"` // optional for temporal events
	Reference             string                  `json:"reference"`
	TargetEntityReference *string                 `json:"target_entity_reference,omitempty"` // for relationships only
	Properties            map[string]PropertyPair `json:"properties"`
}

// PropertyPair represents the old/new value pair for any property
type PropertyPair struct {
	Old interface{} `json:"old"`
	New interface{} `json:"new"`
}

// EventHeaders for Kafka message headers to enable filtering
type EventHeaders struct {
	EntityType    string `header:"entity_type"`
	DataType      string `header:"data_type"`
	Operation     string `header:"operation"`
	SourceService string `header:"source_service"`
	SchemaVersion string `header:"schema_version"`
	FieldsChanged string `header:"fields_changed,omitempty"`
	TableName     string `header:"table_name"`
}

// Constants for event types and operations
const (
	// Event Types
	EventTypeEntityCreated           = "entity_created"
	EventTypeEntityPropertiesUpdated = "entity_properties_updated"
	EventTypeEntityDeleted           = "entity_deleted"
	EventTypeRelationshipCreated     = "relationship_created"
	EventTypeRelationshipUpdated     = "relationship_updated"
	EventTypeRelationshipDeleted     = "relationship_deleted"
	EventTypeTemporalDataCreated     = "temporal_data_created"
	EventTypeTemporalDataUpdated     = "temporal_data_updated"
	EventTypeTemporalDataDeleted     = "temporal_data_deleted"

	// CDC Operations (for mapping)
	OperationInsert = "INSERT"
	OperationUpdate = "UPDATE"
	OperationDelete = "DELETE"

	// Tables
	TableEntities           = "entities"
	TableEdges              = "edges"
	TableTemporalProperties = "temporal_properties"
)
