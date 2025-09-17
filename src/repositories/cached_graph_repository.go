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

type CachedGraphRepository struct {
	graphQueryRepository *GraphQueryRepository
	redisClient          *redis.RedisClient
}

type CacheableResult struct {
	GraphNodes    []domain.GraphNode          `json:"graph_nodes"`
	TemporalProps []entities.TemporalProperty `json:"temporal_props"`
}

func NewCachedGraphRepository(
	graphQueryRepository *GraphQueryRepository,
	redisClient *redis.RedisClient,
) *CachedGraphRepository {
	return &CachedGraphRepository{
		graphQueryRepository: graphQueryRepository,
		redisClient:          redisClient,
	}
}

func (r *CachedGraphRepository) QueryTree(
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

func (r *CachedGraphRepository) generateCacheKey(
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

func (r *CachedGraphRepository) getFromCache(ctx context.Context, cacheKey string) (*CacheableResult, bool, error) {

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

func (r *CachedGraphRepository) setInCache(
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

func (r *CachedGraphRepository) InvalidateByEntityIDs(ctx context.Context, entityIDs []int64) error {
	if len(entityIDs) == 0 {
		return nil
	}

	registryKeys := make([]string, len(entityIDs))
	for i, entityID := range entityIDs {
		registryKeys[i] = fmt.Sprintf("registry:entity:%d", entityID)
	}

	registryResults, err := r.redisClient.GetMultipleSetMembers(ctx, registryKeys)
	if err != nil {
		return fmt.Errorf("failed to get registry data: %w", err)
	}

	allKeysToDelete := make(map[string]bool)

	for registryKey, relatedKeys := range registryResults {
		// Adicionar o próprio registry
		allKeysToDelete[registryKey] = true

		// Adicionar todas as chaves relacionadas
		for _, relatedKey := range relatedKeys {
			allKeysToDelete[relatedKey] = true
		}
	}

	keysToDelete := make([]string, 0, len(allKeysToDelete))
	for key := range allKeysToDelete {
		keysToDelete = append(keysToDelete, key)
	}

	// 5. Deletar todas as chaves
	if len(keysToDelete) > 0 {
		log.Printf("Invalidating %d cache keys for %d entities", len(keysToDelete), len(entityIDs))
		return r.redisClient.InvalidateEntity(ctx, keysToDelete)
	}

	return nil
}

func (r *CachedGraphRepository) removeDuplicates(keys []string) []string {
	keySet := make(map[string]bool)
	for _, key := range keys {
		keySet[key] = true
	}

	unique := make([]string, 0, len(keySet))
	for key := range keySet {
		unique = append(unique, key)
	}

	return unique
}
