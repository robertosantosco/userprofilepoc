package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
	"userprofilepoc/src/adapters/kafka/consumers"
	"userprofilepoc/src/helper/env"
	"userprofilepoc/src/infra/kafka"
	"userprofilepoc/src/infra/postgres"
	"userprofilepoc/src/infra/redis"
	"userprofilepoc/src/repositories"

	"go.uber.org/fx"
)

func main() {
	log.SetOutput(os.Stdout)
	log.Println("Starting Entities/Edges Consumer with Uber Fx...")

	app := fx.New(
		// Providers
		fx.Provide(
			newLogger,
			newReadWriteClient,
			newRedisClient,
			newKafkaClient,
			newCachedGraphRepository,
			newGraphWriteRepository,
			newEntitiesEdgesConsumer,
		),

		// Invocations
		fx.Invoke(startConsumer),
	)

	// Start the application
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.Start(ctx); err != nil {
		log.Fatalf("Failed to start consumer application: %v", err)
	}

	// Wait for interrupt signal to gracefully shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Println("Shutting down entities/edges consumer...")

	// Stop the application
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()

	if err := app.Stop(stopCtx); err != nil {
		log.Printf("Failed to stop application gracefully: %v", err)
	}

	log.Println("Entities/edges consumer shutdown complete")
}

func newLogger() *slog.Logger {
	logLevel := env.GetString("LOG_LEVEL", "info")
	var level slog.Level

	switch logLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	logger := slog.New(h)
	slog.SetDefault(logger)
	return logger
}

func newReadWriteClient() (*postgres.ReadWriteClient, error) {
	dbReadHost := env.MustGetString("DB_READ_HOST")
	dbWriteHost := env.MustGetString("DB_WRITE_HOST")
	dbReadPort := env.GetString("DB_READ_PORT", "5432")
	dbWritePort := env.GetString("DB_WRITE_PORT", "5432")
	dbname := env.MustGetString("DB_NAME")
	dbUser := env.MustGetString("DB_USER")
	dbPassword := env.MustGetString("DB_PASSWORD")
	maxConnections := env.GetInt("DB_MAX_POOL_CONNECTIONS", 25)

	return postgres.NewReadWriteClient(dbReadHost, dbWriteHost, dbReadPort, dbWritePort, dbname, dbUser, dbPassword, maxConnections)
}

func newRedisClient() *redis.RedisClient {
	redisHosts := env.MustGetString("REDIS_HOSTS")
	redisPoolSize := env.GetInt("REDIS_POOL_SIZE", 50)
	redisDefaultTTLSeconds := env.GetInt("REDIS_DEFAULT_TTL_SECONDS", 120)
	redisDefaultTTL := time.Duration(redisDefaultTTLSeconds) * time.Second

	return redis.NewRedisClient(redisHosts, redisPoolSize, redisDefaultTTL)
}

func newKafkaClient() (*kafka.KafkaClient, error) {
	brokers := env.MustGetString("KAFKA_BROKERS")
	groupID := env.MustGetString("KAFKA_ENTITIES_EDGES_CONSUMER_GROUP_ID")
	batchSize := env.MustGetInt("KAFKA_BATCH_SIZE")

	return kafka.NewKafkaClient(brokers, groupID, batchSize)
}

func newCachedGraphRepository(
	redisClient *redis.RedisClient,
) *repositories.CachedGraphRepository {
	return repositories.NewCachedGraphRepository(nil, redisClient)
}

func newGraphWriteRepository(
	readWriteClient *postgres.ReadWriteClient,
	cachedGraphRepository *repositories.CachedGraphRepository,
) *repositories.GraphWriteRepository {
	return repositories.NewGraphWriteRepository(readWriteClient.GetWritePool(), cachedGraphRepository)
}

func newEntitiesEdgesConsumer(
	logger *slog.Logger,
	graphWriteRepository *repositories.GraphWriteRepository,
) *consumers.EntitiesEdgesConsumer {
	return consumers.NewEntitiesEdgesConsumer(logger, graphWriteRepository)
}

func startConsumer(
	lc fx.Lifecycle,
	logger *slog.Logger,
	kafkaClient *kafka.KafkaClient,
	entitiesConsumer *consumers.EntitiesEdgesConsumer,
) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			topic := env.GetString("KAFKA_ENTITIES_EDGES_CONSUMER_TOPIC")
			logger.Info("Starting entities/edges consumer", "topic", topic)

			// Start consumer in background
			go func() {
				if err := entitiesConsumer.Start(ctx, kafkaClient, topic); err != nil {
					logger.Error("Consumer failed", "error", err)
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Shutting down Kafka client...")
			if err := kafkaClient.Close(); err != nil {
				logger.Error("Failed to close Kafka client", "error", err)
				return err
			}
			logger.Info("Kafka client shut down gracefully")
			return nil
		},
	})
}
