package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"
	"userprofilepoc/src/helper/env"
	"userprofilepoc/src/infra/postgres"
	"userprofilepoc/src/infra/redis"
	"userprofilepoc/src/repositories"
	"userprofilepoc/src/server"
	"userprofilepoc/src/services"

	"go.uber.org/fx"
)

func main() {
	// Configurar logger
	log.SetOutput(os.Stdout)
	log.Println("Starting API server with Uber Fx...")

	app := fx.New(
		// Providers
		fx.Provide(
			newLogger,
			newNewReadWriteClient,
			newRedisClient,
			newServer,
			newGraphQueryRepository,
			newCachedGraphRepository,
			newGraphWriteRepository,
			newEntitiesService,
			newTemporalWriteRepository,
			newTemporalDataService,
		),

		// Invocations
		fx.Invoke(registerServerHooks),
	)

	// Start the application
	if err := app.Start(context.Background()); err != nil {
		log.Fatalf("Failed to start application: %v", err)
	}

	// Wait for app to exit gracefully
	<-app.Done()
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

// newNewReadWriteClient configures and returns a pgxpool connection pool
func newNewReadWriteClient() (*postgres.ReadWriteClient, error) {
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

func newGraphQueryRepository(readWriteClient *postgres.ReadWriteClient) *repositories.GraphQueryRepository {
	return repositories.NewGraphQueryRepository(readWriteClient.GetReadPool())
}

func newCachedGraphRepository(
	graphQueryRepository *repositories.GraphQueryRepository,
	redisClient *redis.RedisClient,
) *repositories.CachedGraphRepository {
	return repositories.NewCachedGraphRepository(graphQueryRepository, redisClient)
}

func newGraphWriteRepository(
	readWriteClient *postgres.ReadWriteClient,
	cachedGraphRepository *repositories.CachedGraphRepository,
) *repositories.GraphWriteRepository {
	return repositories.NewGraphWriteRepository(readWriteClient.GetWritePool(), cachedGraphRepository)
}

func newEntitiesService(
	cachedGraphRepository *repositories.CachedGraphRepository,
	graphWriteRepository *repositories.GraphWriteRepository,
) *services.GraphService {
	return services.NewGraphService(cachedGraphRepository, graphWriteRepository)
}

func newTemporalWriteRepository(
	readWriteClient *postgres.ReadWriteClient,
	cachedGraphRepository *repositories.CachedGraphRepository,
) *repositories.TemporalWriteRepository {
	return repositories.NewTemporalWriteRepository(readWriteClient.GetWritePool(), cachedGraphRepository)
}

func newTemporalDataService(temporalWriteRepository *repositories.TemporalWriteRepository) *services.TemporalDataService {
	return services.NewTemporalDataService(temporalWriteRepository)
}

func newServer(
	logger *slog.Logger,
	graphService *services.GraphService,
	temporalDataService *services.TemporalDataService,
) *server.Server {

	port := 8888 // default value
	if portStr := os.Getenv("SERVER_ADDR"); portStr != "" {
		if val, err := strconv.Atoi(portStr); err == nil {
			port = val
		}
	}

	server := server.NewServer(logger, port, graphService, temporalDataService)

	return server
}

// registerServerHooks registers lifecycle hooks for the HTTP server
func registerServerHooks(lc fx.Lifecycle, srv *server.Server) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// Start server in a separate goroutine
			go func() {
				if err := srv.Start(); err != nil && err != http.ErrServerClosed {
					log.Fatalf("Server failed: %v", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			// Create timeout context for graceful shutdown
			shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			log.Println("Shutting down server...")
			if err := srv.Shutdown(shutdownCtx); err != nil {
				log.Printf("Server forced to shutdown: %v", err)
				return err
			}
			log.Println("Server exited gracefully")
			return nil
		},
	})
}
