package graph

import (
	"context"
	"fmt"
	"time"
	"userprofilepoc/src/domain"
	"userprofilepoc/src/repositories"
)

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
