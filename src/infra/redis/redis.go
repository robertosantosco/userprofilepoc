package redis

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	client            *redis.ClusterClient
	defaultTTLSeconds time.Duration
	keyPrefix         string
}

func NewRedisClient(addrs string, poolSize int, defaultTTLSeconds time.Duration) *RedisClient {
	client := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs: strings.Split(addrs, ","),

		// Pool settings para alta concorrência
		PoolSize:     poolSize,
		MinIdleConns: 10,

		// Cluster específico
		MaxRedirects: 3,

		// Timeouts otimizados para cache
		DialTimeout:  5 * time.Second,
		ReadTimeout:  1 * time.Second,
		WriteTimeout: 1 * time.Second,

		// Retry e circuit breaker
		MaxRetries:      3,
		MinRetryBackoff: 50 * time.Millisecond,
		MaxRetryBackoff: 500 * time.Millisecond,
	})

	return &RedisClient{
		client:            client,
		defaultTTLSeconds: defaultTTLSeconds,
		keyPrefix:         "",
	}
}

// WithPrefix retorna uma nova instância do RedisClient com prefixo nas chaves
// Útil para isolar ambientes de teste
func (rc *RedisClient) WithPrefix(prefix string) *RedisClient {
	return &RedisClient{
		client:            rc.client,
		defaultTTLSeconds: rc.defaultTTLSeconds,
		keyPrefix:         prefix,
	}
}

// prefixKey adiciona o prefixo à chave se configurado
func (rc *RedisClient) prefixKey(key string) string {
	if rc.keyPrefix == "" {
		return key
	}
	return rc.keyPrefix + key
}

func (rc *RedisClient) SetKey(ctx context.Context, key string, value string) error {
	key = rc.prefixKey(key)
	fields := map[string]interface{}{
		"data":      value,
		"cached_at": time.Now().Unix(),
	}

	err := rc.client.HSet(ctx, key, fields).Err()
	if err != nil {
		return err
	}

	return rc.client.Expire(ctx, key, rc.defaultTTLSeconds).Err()
}

func (rc *RedisClient) SetMultiple(ctx context.Context, keyValues map[string]string) error {
	pipe := rc.client.Pipeline()

	for key, value := range keyValues {
		prefixedKey := rc.prefixKey(key)
		pipe.HSet(ctx, prefixedKey, value)
		pipe.Expire(ctx, prefixedKey, rc.defaultTTLSeconds)
	}

	_, err := pipe.Exec(ctx)
	return err
}

func (rc *RedisClient) SetWithRegistry(ctx context.Context, cacheKey string, cacheValue string, registryKeys []string) error {
	pipe := rc.client.Pipeline()

	// 1. Set do cache principal
	cacheKey = rc.prefixKey(cacheKey)
	fields := map[string]interface{}{
		"data":      cacheValue,
		"cached_at": time.Now().Unix(),
	}
	pipe.HSet(ctx, cacheKey, fields)
	pipe.Expire(ctx, cacheKey, rc.defaultTTLSeconds)

	for _, registryKey := range registryKeys {
		prefixedRegistryKey := rc.prefixKey(registryKey)
		pipe.SAdd(ctx, prefixedRegistryKey, cacheKey)
		pipe.Expire(ctx, prefixedRegistryKey, rc.defaultTTLSeconds)
	}

	_, err := pipe.Exec(ctx)
	return err
}

func (rc *RedisClient) GetKey(ctx context.Context, key string) (string, bool, error) {
	key = rc.prefixKey(key)
	result := rc.client.HGet(ctx, key, "data")

	// Cache miss
	if result.Err() == redis.Nil {
		return "", false, nil
	}
	if result.Err() != nil {
		return "", false, result.Err()
	}

	return result.Val(), true, nil
}

func (rc *RedisClient) GetMultipleSetMembers(ctx context.Context, keys []string) (map[string][]string, error) {
	if len(keys) == 0 {
		return make(map[string][]string), nil
	}

	pipe := rc.client.Pipeline()

	// Adicionar todos os SMEMBERS ao pipeline
	cmds := make(map[string]*redis.StringSliceCmd)
	for _, key := range keys {
		prefixedKey := rc.prefixKey(key)
		cmds[key] = pipe.SMembers(ctx, prefixedKey)
	}

	// Executar pipeline de uma vez
	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}

	// Coletar resultados
	results := make(map[string][]string)
	for key, cmd := range cmds {
		members, err := cmd.Result()
		if err == redis.Nil {
			results[key] = []string{} // Set vazio
		} else if err != nil {
			log.Printf("Failed to get members for key %s: %v", key, err)
			results[key] = []string{}
		} else {
			results[key] = members
		}
	}

	return results, nil
}

// Invalidação em cluster requer cuidado especial
func (rc *RedisClient) InvalidateEntity(ctx context.Context, keys []string) error {
	var errors []string

	for _, key := range keys {
		prefixedKey := rc.prefixKey(key)
		if err := rc.client.Del(ctx, prefixedKey).Err(); err != nil {
			errors = append(errors, fmt.Sprintf("key %s: %v", key, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("invalidation errors: %s", strings.Join(errors, "; "))
	}

	return nil
}

// FlushByPrefix deleta todas as chaves que começam com o prefixo configurado
// ATENÇÃO: Use apenas em ambientes de teste
func (rc *RedisClient) FlushByPrefix(ctx context.Context) error {
	if rc.keyPrefix == "" {
		return fmt.Errorf("cannot flush without prefix - safety check")
	}

	pattern := rc.keyPrefix + "*"
	var cursor uint64
	var deletedCount int

	for {
		keys, nextCursor, err := rc.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("failed to scan keys: %w", err)
		}

		if len(keys) > 0 {
			if err := rc.client.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("failed to delete keys: %w", err)
			}
			deletedCount += len(keys)
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	log.Printf("Flushed %d keys with prefix '%s'", deletedCount, rc.keyPrefix)
	return nil
}

// Health check para o cluster
func (rc *RedisClient) HealthCheck(ctx context.Context) error {
	return rc.client.Ping(ctx).Err()
}
