package services

import (
	"context"
	"fmt"
	"time"
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

// obtem o nodeTree pelo ID da entidade raiz.
func (gs *GraphService) GetTreeByEntityID(ctx context.Context, EntityID int64, depthLimit int, startTime time.Time) (*domain.NodeTree, error) {

	condition := repositories.FindCondition{
		Field:    "id",
		Operator: "=",
		Value:    EntityID,
	}

	graphNodes, temporalProps, err := gs.cachedGraphRepository.QueryTree(ctx, condition, depthLimit, startTime)
	if err != nil {
		return nil, fmt.Errorf("GraphService.GetTreeByEntityID - failed to QueryTree from repository: %w", err)
	}

	_, specificTrees := gs.buildNodeTrees(graphNodes, temporalProps, []int64{EntityID})

	if len(specificTrees) == 0 {
		return nil, fmt.Errorf("GraphService.GetTreeByEntityID - root node (%d) could not be found after assembly: %w", EntityID, domain.ErrEntityNotFound)
	}

	if len(specificTrees) > 1 {
		return nil, fmt.Errorf("GraphService.GetTreeByEntityID - returned more than one: %d", len(specificTrees))
	}

	return specificTrees[0], nil
}

// obtem todas as nodeTrees pela propriedade das entidades raiz.
func (gs *GraphService) GetTreesByEntityProperty(ctx context.Context, propName string, propValue string, depthLimit int, startTime time.Time) ([]*domain.NodeTree, error) {

	condition := repositories.FindCondition{
		Field:    propName,
		Operator: "@>",
		Value:    propValue,
	}

	graphNodes, temporalProps, err := gs.cachedGraphRepository.QueryTree(ctx, condition, depthLimit, startTime)
	if err != nil {
		return nil, fmt.Errorf("GraphService.GetTreesByEntityProperty - failed to QueryTree from repository: %w", err)
	}

	naturalRoots, _ := gs.buildNodeTrees(graphNodes, temporalProps, nil)

	if len(naturalRoots) == 0 {
		return nil, fmt.Errorf("GraphService.GetTreesByEntityProperty - no entities found for property %s = %s: %w", propName, propValue, domain.ErrEntityNotFound)
	}

	return naturalRoots, nil
}

// GetTreesByEntityIDs obtém múltiplas árvores por IDs de entidade de forma eficiente
func (gs *GraphService) GetTreesByEntityIDs(ctx context.Context, entityIDs []int64, depthLimit int, startTime time.Time) []*domain.NodeTree {
	if len(entityIDs) == 0 {
		return []*domain.NodeTree{}
	}

	condition := repositories.FindCondition{
		Field:    "id",
		Operator: "IN",
		Value:    entityIDs,
	}

	graphNodes, temporalProps, err := gs.cachedGraphRepository.QueryTree(ctx, condition, depthLimit, startTime)
	if err != nil {
		fmt.Println(err)
		return []*domain.NodeTree{}
	}

	_, specificTrees := gs.buildNodeTrees(graphNodes, temporalProps, entityIDs)

	return specificTrees
}

// atualiza entities e edges.
func (s *GraphService) SyncGraph(ctx context.Context, request domain.SyncGraphRequest) error {
	if len(request.Entities) == 0 && len(request.Relationships) == 0 {
		return fmt.Errorf("sync request must contain at least one entity or relationship")
	}

	return s.graphWriteRepository.SyncGraph(ctx, request)
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
