package services

import (
	"context"
	"fmt"
	"userprofilepoc/src/domain"
	"userprofilepoc/src/repositories"
)

type TemporalDataService struct {
	temporalWriteRepository *repositories.TemporalWriteRepository
}

func NewTemporalDataService(temporalWriteRepository *repositories.TemporalWriteRepository) *TemporalDataService {
	return &TemporalDataService{temporalWriteRepository: temporalWriteRepository}
}

func (s *TemporalDataService) UpsertDataPoints(ctx context.Context, request domain.SyncTemporalPropertyRequest) error {
	if len(request.DataPoints) == 0 {
		return fmt.Errorf("ingestion request must contain at least one data point")
	}

	return s.temporalWriteRepository.UpsertDataPoints(ctx, request)
}
