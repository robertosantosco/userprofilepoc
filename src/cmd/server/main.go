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
	"userprofilepoc/src/repositories"
	"userprofilepoc/src/server"
	"userprofilepoc/src/services"

	"github.com/jackc/pgx/v5/pgxpool"
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
			newSQLClient,
			newServer,
			newGraphQueryRepository,
			newEntitiesService,
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

// newSQLClient configures and returns a pgxpool connection pool
func newSQLClient() (*pgxpool.Pool, error) {
	dbHost := env.MustGetString("DB_HOST")
	dbPort := env.GetString("DB_PORT", "5432")
	dbname := env.MustGetString("DB_NAME")
	dbUser := env.MustGetString("DB_USER")
	dbPassword := env.MustGetString("DB_PASSWORD")
	maxConnections := env.GetInt("DB_MAX_POOL_CONNECTIONS", 25)

	return postgres.NewPostgresClient(dbHost, dbPort, dbname, dbUser, dbPassword, maxConnections)
}

func newGraphQueryRepository(pool *pgxpool.Pool) *repositories.GraphQueryRepository {
	return repositories.NewGraphQueryRepository(pool)
}

func newEntitiesService(graphQueryRepository *repositories.GraphQueryRepository) *services.GraphService {
	return services.NewGraphService(graphQueryRepository)
}

func newServer(
	logger *slog.Logger,
	graphService *services.GraphService,
) *server.Server {

	port := 8888 // default value
	if portStr := os.Getenv("SERVER_ADDR"); portStr != "" {
		if val, err := strconv.Atoi(portStr); err == nil {
			port = val
		}
	}

	server := server.NewServer(logger, port, graphService)

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
