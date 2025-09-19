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

	rootNode, err := gs.buildNodeTree(graphNodes, temporalProps)

	// 4. Encontrar e retornar o nó raiz.
	if err != nil {
		return nil, fmt.Errorf("GraphService.GetTreeByEntityID - root node (%d) could not be found after assembly: %w", EntityID, err)
	}

	return rootNode, nil
}

// obtem o nodeTree pela propriedade da entidade raiz.
func (gs *GraphService) GetTreeByEntityProperty(ctx context.Context, propName string, propValue string, depthLimit int, startTime time.Time) (*domain.NodeTree, error) {

	condition := repositories.FindCondition{
		Field:    propName,
		Operator: "@>",
		Value:    propValue,
	}

	graphNodes, temporalProps, err := gs.cachedGraphRepository.QueryTree(ctx, condition, depthLimit, startTime)
	if err != nil {
		return nil, fmt.Errorf("GraphService.GetTreeByEntityProperty - failed to QueryTree from repository: %w", err)
	}

	// 2. Chama o repositório.
	rootNode, err := gs.buildNodeTree(graphNodes, temporalProps)
	if err != nil {
		return nil, fmt.Errorf("GraphService.GetTreeByEntityProperty - failed to build profile for document %s %s: %w", propName, propValue, err)
	}

	return rootNode, nil
}

// atualiza entities e edges.
func (s *GraphService) SyncGraph(ctx context.Context, request domain.SyncGraphRequest) error {
	if len(request.Entities) == 0 && len(request.Relationships) == 0 {
		return fmt.Errorf("sync request must contain at least one entity or relationship")
	}

	return s.graphWriteRepository.SyncGraph(ctx, request)
}

// Retorna o nó raiz da árvore montada ou um erro se a raiz não puder ser determinada.
func (es *GraphService) buildNodeTree(graphNodes []domain.GraphNode, temporalProps []entities.TemporalProperty) (*domain.NodeTree, error) {
	if len(graphNodes) == 0 {
		return nil, fmt.Errorf("graphNodes is empty; cannot build tree")
	}

	// Passo 1: Organizar dados temporais em um mapa para acesso rápido O(1).
	temporalMap := make(map[int64][]entities.TemporalProperty)
	for _, prop := range temporalProps {
		temporalMap[prop.EntityID] = append(temporalMap[prop.EntityID], prop)
	}

	// Passo 2: Criar todos os nós da árvore de perfil em um mapa para acesso O(1).
	nodes := make(map[int64]*domain.NodeTree, len(graphNodes))
	for i := range graphNodes {
		nodeData := graphNodes[i]

		nodes[nodeData.ID] = &domain.NodeTree{
			Entity:       nodeData.Entity,
			TemporalData: temporalMap[nodeData.ID],
			Edges:        make([]*domain.ProfileEdge, 0),
		}
	}

	// Passo 3: Conectar os nós na estrutura de árvore e identificar a raiz.
	var rootNode *domain.NodeTree

	for _, nodeData := range graphNodes {
		// A raiz da nossa consulta é o único nó que não tem pais DENTRO do resultado do grafo.
		if nodeData.ParentsInfo == nil || len(nodeData.ParentsInfo) == 0 {
			rootNode = nodes[nodeData.ID]
		}

		if len(nodeData.ParentsInfo) > 0 {
			childNode := nodes[nodeData.ID]
			for _, parentInfo := range nodeData.ParentsInfo {

				if parentNode, ok := nodes[parentInfo.ParentID]; ok {
					parentNode.Edges = append(parentNode.Edges, &domain.ProfileEdge{
						Type:   parentInfo.Type,
						Entity: childNode,
					})
				}
			}
		}
	}

	// Passo 4: Verificação de sanidade. Se percorremos todos os nós e não encontramos uma raiz,
	if rootNode == nil {
		return nil, fmt.Errorf("could not determine the root node from the provided graph data; a cycle may exist or the root is missing")
	}

	return rootNode, nil
}
