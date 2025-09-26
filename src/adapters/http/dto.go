package http

import (
	"encoding/json"
	"time"
	"userprofilepoc/src/domain"
	"userprofilepoc/src/domain/entities"
)

type NodeTreeDTO struct {
	ID         int64           `json:"id"`
	Type       string          `json:"type"`
	Reference  string          `json:"reference"`
	Properties json.RawMessage `json:"properties"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`

	Edges        []*NodeTreeEdgeDTO            `json:"edges"`
	TemporalData map[string][]*TemporalItemDTO `json:"temporal_data"`
}

type NodeTreeEdgeDTO struct {
	Type   string       `json:"type"`
	Entity *NodeTreeDTO `json:"entity"`
}

type TemporalItemDTO struct {
	Value         json.RawMessage      `json:"value"`
	Granularity   entities.Granularity `json:"granularity"`
	ReferenceDate time.Time            `json:"reference_date"`
	CreatedAt     time.Time            `json:"created_at"`
	UpdatedAt     time.Time            `json:"updated_at"`
}

// // MarshalJSON customizado para "achatar" o campo Properties.
// func (dto NodeTreeDTO) MarshalJSON() ([]byte, error) {
// 	tempMap := make(map[string]interface{})

// 	tempMap["id"] = dto.ID
// 	tempMap["type"] = dto.Type
// 	tempMap["reference"] = dto.Reference
// 	tempMap["created_at"] = dto.CreatedAt
// 	tempMap["updated_at"] = dto.UpdatedAt
// 	tempMap["edges"] = dto.Edges

// 	// Desserializamos o `Properties` (que é um JSON) em um mapa genérico.
// 	if len(dto.Properties) > 0 && string(dto.Properties) != "null" {
// 		var propsMap map[string]interface{}
// 		if err := json.Unmarshal(dto.Properties, &propsMap); err != nil {
// 			return nil, err
// 		}

// 		// Iteramos sobre o mapa de propriedades e adicionamos cada par chave-valor ao nosso mapa principal.
// 		for key, value := range propsMap {
// 			tempMap[key] = value
// 		}
// 	}

// 	return json.Marshal(tempMap)
// }

type BatchGraphRequest struct {
	EntityIDs  []int64 `json:"entity_ids" binding:"required"`
	DepthLimit *int    `json:"depth_limit,omitempty"`
	StartTime  *string `json:"start_time,omitempty"`
}

func MapDomainToResponse(node *domain.NodeTree) *NodeTreeDTO {
	if node == nil {
		return nil
	}

	//  LÓGICA DE AGREGAÇÃO TEMPORAL
	var aggregatedTemporalData map[string][]*TemporalItemDTO
	if len(node.TemporalData) > 0 {
		aggregatedTemporalData = make(map[string][]*TemporalItemDTO)

		for _, prop := range node.TemporalData {
			item := &TemporalItemDTO{
				Value:         prop.Value,
				Granularity:   prop.Granularity,
				ReferenceDate: prop.ReferenceDate,
				CreatedAt:     prop.CreatedAt,
				UpdatedAt:     prop.UpdatedAt,
			}

			aggregatedTemporalData[prop.Key] = append(aggregatedTemporalData[prop.Key], item)
		}
	}

	nodeTreeDTO := &NodeTreeDTO{
		ID:           node.ID,
		Type:         node.Type,
		Reference:    node.Reference,
		Properties:   node.Properties,
		CreatedAt:    node.CreatedAt,
		UpdatedAt:    node.UpdatedAt,
		TemporalData: aggregatedTemporalData,
		Edges:        make([]*NodeTreeEdgeDTO, 0, len(node.Edges)),
	}

	for _, domainEdge := range node.Edges {
		nodeTreeEdgeDTO := &NodeTreeEdgeDTO{
			Type:   domainEdge.Type,
			Entity: MapDomainToResponse(domainEdge.Entity),
		}

		nodeTreeDTO.Edges = append(nodeTreeDTO.Edges, nodeTreeEdgeDTO)
	}

	return nodeTreeDTO
}
