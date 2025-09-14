package server

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
	logger        *slog.Logger
	server        *http.Server
	mux           *http.ServeMux
	port          int
	entityService *services.EntityService
}

// NewServer cria uma nova instância do servidor
func NewServer(
	logger *slog.Logger,
	port int,
	entityService *services.EntityService,
) *Server {
	server := &Server{
		mux:           http.NewServeMux(),
		port:          port,
		logger:        logger,
		entityService: entityService,
	}

	server.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      server.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	server.mux.HandleFunc("GET /v1/entities/{id}", server.GetEntityByID)
	server.mux.HandleFunc("GET /v1/entities/by-properties/{prop}/value/{value}", server.GetEntityByProperty)

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
