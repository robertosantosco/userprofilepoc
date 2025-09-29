package graph

import (
	"context"
	"fmt"
	"time"
	"userprofilepoc/src/domain"
	"userprofilepoc/src/repositories"
)

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
