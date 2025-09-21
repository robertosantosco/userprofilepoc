package http

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"userprofilepoc/src/services"

	"time"
)

// Server representa o servidor HTTP da API
type Server struct {
	logger              *slog.Logger
	server              *http.Server
	mux                 *http.ServeMux
	port                int
	graphService        *services.GraphService
	temporalDataService *services.TemporalDataService
}

// NewServer cria uma nova instância do servidor
func NewServer(
	logger *slog.Logger,
	port int,
	graphService *services.GraphService,
	temporalDataService *services.TemporalDataService,
) *Server {
	server := &Server{
		mux:                 http.NewServeMux(),
		port:                port,
		logger:              logger,
		graphService:        graphService,
		temporalDataService: temporalDataService,
	}

	server.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      server.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Rotas de Leitura
	server.mux.HandleFunc("GET /v1/graph/{id}", server.GetGraphByID)
	server.mux.HandleFunc("GET /v1/graph/by-property/{prop}/value/{value}", server.GetGraphByProperty)

	// Rotas de Escritas
	server.mux.HandleFunc("POST /v1/graph/sync", server.SyncGraph)

	//
	server.mux.HandleFunc("POST /v1/temporal/ingest", server.IngestTemporalData)

	return server
}

// Start inicia o servidor HTTP
func (s *Server) Start() error {
	s.logger.Info("Server started", "port", s.port)

	return s.server.ListenAndServe()
}

// Shutdown encerra o servidor HTTP de forma graciosa
func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down server...")
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}
