package repositories

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"userprofilepoc/src/domain"
	"userprofilepoc/src/domain/entities"
	"userprofilepoc/src/infra/redis"
)

type CachedGraphQueryRepository struct {
	graphQueryRepository *GraphQueryRepository
	redisClient          *redis.RedisClient
}

type CacheableResult struct {
	GraphNodes    []domain.GraphNode          `json:"graph_nodes"`
	TemporalProps []entities.TemporalProperty `json:"temporal_props"`
}

func NewCachedGraphQueryRepository(
	graphQueryRepository *GraphQueryRepository,
	redisClient *redis.RedisClient,
) *CachedGraphQueryRepository {
	return &CachedGraphQueryRepository{
		graphQueryRepository: graphQueryRepository,
		redisClient:          redisClient,
	}
}

func (r *CachedGraphQueryRepository) QueryTree(
	ctx context.Context,
	condition FindCondition,
	depthLimit int,
	referenceMonth time.Time,
) ([]domain.GraphNode, []entities.TemporalProperty, error) {
	cacheKey := r.generateCacheKey(condition, depthLimit, referenceMonth)

	cachedData, found, err := r.getFromCache(ctx, cacheKey)
	if found && err == nil {
		log.Printf("Cache HIT for key: %s", cacheKey)
		return cachedData.GraphNodes, cachedData.TemporalProps, nil
	}

	if err != nil {
		// Log erro de cache mas continua com PostgreSQL
		log.Printf("Cache error for key %s: %v", cacheKey, err)
	}

	log.Printf("Cache MISS for key: %s", cacheKey)

	graphNodes, temporalProps, err := r.graphQueryRepository.QueryTree(ctx, condition, depthLimit, referenceMonth)
	if err != nil {
		return nil, nil, fmt.Errorf("postgres query failed: %w", err)
	}

	go func() {
		// Timeout de 30 segundos para operação de cache
		ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		r.setInCache(ctxWithTimeout, cacheKey, graphNodes, temporalProps)
	}()

	return graphNodes, temporalProps, nil
}

func (r *CachedGraphQueryRepository) generateCacheKey(
	condition FindCondition,
	depthLimit int,
	referenceMonth time.Time,
) string {
	// Criar string única com todos os parâmetros
	keyData := fmt.Sprintf("query:%s:%s:%v:depth:%d:month:%s",
		condition.Field,
		condition.Operator,
		condition.Value,
		depthLimit,
		referenceMonth.Format("2006-01"),
	)

	// Hash para chave mais limpa e consistente
	hash := md5.Sum([]byte(keyData))
	return fmt.Sprintf("graph:tree:%x", hash)
}

func (r *CachedGraphQueryRepository) getFromCache(ctx context.Context, cacheKey string) (*CacheableResult, bool, error) {

	cachedJSON, found, err := r.redisClient.GetKey(ctx, cacheKey)
	if !found || err != nil {
		return nil, found, err
	}

	var result CacheableResult
	if err := json.Unmarshal([]byte(cachedJSON), &result); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal cached data: %w", err)
	}

	return &result, true, nil
}

func (r *CachedGraphQueryRepository) setInCache(
	ctx context.Context,
	cacheKey string,
	graphNodes []domain.GraphNode,
	temporalProps []entities.TemporalProperty,
) {
	cacheData := CacheableResult{
		GraphNodes:    graphNodes,
		TemporalProps: temporalProps,
	}

	dataJSON, err := json.Marshal(cacheData)
	if err != nil {
		log.Printf("Failed to marshal cache data for key %s: %v", cacheKey, err)
		return
	}

	registryKeys := make([]string, len(graphNodes))
	for i, node := range graphNodes {
		registryKeys[i] = fmt.Sprintf("registry:entity:%d", node.ID)
	}

	err = r.redisClient.SetWithRegistry(ctx, cacheKey, string(dataJSON), registryKeys)
	if err != nil {
		log.Printf("Failed to set cache with registry for key %s: %v", cacheKey, err)
		return
	}

	log.Printf("Cache SET with registry for key: %s (%d entities)", cacheKey, len(graphNodes))
}
