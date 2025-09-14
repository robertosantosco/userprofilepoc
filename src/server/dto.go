package server

import (
	"encoding/json"
	"time"
	"userprofilepoc/src/domain"
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
	Value       json.RawMessage `json:"value"`
	Period      PeriodDTO       `json:"period"`
	Granularity string          `json:"granularity"`
	StartTS     time.Time       `json:"start_ts"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type PeriodDTO struct {
	Start *time.Time `json:"start"`
	End   *time.Time `json:"end"`
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
				Value: prop.Value,
				Period: PeriodDTO{
					Start: &prop.PeriodStart,
					End:   prop.PeriodEnd,
				},
				Granularity: prop.Granularity,
				StartTS:     prop.StartTS,
				CreatedAt:   prop.CreatedAt,
				UpdatedAt:   prop.UpdatedAt,
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
