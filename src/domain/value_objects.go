package domain

import (
	"encoding/json"
	"errors"
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
	Reference  string
	Type       string
	Properties json.RawMessage
}

// SyncRelationshipDTO define a estrutura de um relacionamento para sincronização.
type SyncRelationshipDTO struct {
	SourceReference  string
	TargetReference  string
	RelationshipType string
}

// SyncGraphRequest é o DTO completo que o serviço usa para solicitar uma sincronização.
type SyncGraphRequest struct {
	Entities      []SyncEntityDTO
	Relationships []SyncRelationshipDTO
}
