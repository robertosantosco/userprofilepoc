package graph

import (
	"context"
	"fmt"
	"time"
	"userprofilepoc/src/domain"
	"userprofilepoc/src/repositories"
)

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
