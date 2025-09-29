package temporal_data

import (
	"userprofilepoc/src/repositories"
)

type TemporalDataService struct {
	temporalWriteRepository *repositories.TemporalWriteRepository
}

func NewTemporalDataService(temporalWriteRepository *repositories.TemporalWriteRepository) *TemporalDataService {
	return &TemporalDataService{temporalWriteRepository: temporalWriteRepository}
}
