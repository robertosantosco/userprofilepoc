package graph

import (
	"userprofilepoc/src/domain"
	"userprofilepoc/src/domain/entities"
	"userprofilepoc/src/repositories"
)

type GraphService struct {
	cachedGraphRepository *repositories.CachedGraphRepository
	graphWriteRepository  *repositories.GraphWriteRepository
}

func NewGraphService(
	cachedGraphRepository *repositories.CachedGraphRepository,
	graphWriteRepository *repositories.GraphWriteRepository,
) *GraphService {
	return &GraphService{
		cachedGraphRepository: cachedGraphRepository,
		graphWriteRepository:  graphWriteRepository,
	}
}

// buildNodeTrees retorna duas listas: roots naturais (n tem pai) e árvores específicas por ID
func (gs *GraphService) buildNodeTrees(graphNodes []domain.GraphNode, temporalProps []entities.TemporalProperty, specificIDs []int64) ([]*domain.NodeTree, []*domain.NodeTree) {
	var naturalRoots []*domain.NodeTree
	var specificTrees []*domain.NodeTree

	if len(graphNodes) == 0 {
		return naturalRoots, specificTrees
	}

	// Organizar dados temporais em um mapa para acesso rápido O(1)
	temporalMap := make(map[int64][]entities.TemporalProperty)
	for _, prop := range temporalProps {
		temporalMap[prop.EntityID] = append(temporalMap[prop.EntityID], prop)
	}

	// Criar todos os nós
	allNodes := make(map[int64]*domain.NodeTree, len(graphNodes))
	for _, nodeData := range graphNodes {
		allNodes[nodeData.ID] = &domain.NodeTree{
			Entity:       nodeData.Entity,
			TemporalData: temporalMap[nodeData.ID],
			Edges:        make([]*domain.ProfileEdge, 0),
		}
	}

	// Conectar edges entre os nós
	for _, nodeData := range graphNodes {
		if len(nodeData.ParentsInfo) > 0 {
			childNode := allNodes[nodeData.ID]
			for _, parentInfo := range nodeData.ParentsInfo {
				if parentNode, ok := allNodes[parentInfo.ParentID]; ok {
					parentNode.Edges = append(parentNode.Edges, &domain.ProfileEdge{
						Type:   parentInfo.Type,
						Entity: childNode,
					})
				}
			}
		}

		// Identificar roots naturais (sem parents no resultado)
		if nodeData.ParentsInfo == nil || len(nodeData.ParentsInfo) == 0 {
			naturalRoots = append(naturalRoots, allNodes[nodeData.ID])
		}
	}

	// Criar árvores específicas na ordem dos IDs solicitados
	for _, specificID := range specificIDs {
		if tree, exists := allNodes[specificID]; exists {
			specificTrees = append(specificTrees, tree)
		}
	}

	return naturalRoots, specificTrees
}
