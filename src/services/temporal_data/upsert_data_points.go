package temporal_data

import (
	"context"
	"fmt"
	"userprofilepoc/src/domain"
)

func (s *TemporalDataService) UpsertDataPoints(ctx context.Context, request domain.SyncTemporalPropertyRequest) error {
	if len(request.DataPoints) == 0 {
		return fmt.Errorf("ingestion request must contain at least one data point")
	}

	return s.temporalWriteRepository.UpsertDataPoints(ctx, request)
}
