package graph

import (
	"context"
	"fmt"
	"userprofilepoc/src/domain"
)

// atualiza entities e edges.
func (s *GraphService) SyncGraph(ctx context.Context, request domain.SyncGraphRequest) error {
	if len(request.Entities) == 0 && len(request.Relationships) == 0 {
		return fmt.Errorf("sync request must contain at least one entity or relationship")
	}

	return s.graphWriteRepository.SyncGraph(ctx, request)
}
