package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"userprofilepoc/src/domain"
)

func (s *Server) GetEntityByID(w http.ResponseWriter, r *http.Request) {
	entityIDStr := r.PathValue("id")
	if entityIDStr == "" {
		http.Error(w, "Entity ID is required", http.StatusBadRequest)
		return
	}

	entityID, err := strconv.ParseInt(entityIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid Entity ID format", http.StatusBadRequest)
		return
	}

	entityTree, err := s.graphService.GetTreeByEntityID(r.Context(), entityID)
	if err != nil {
		if errors.Is(err, domain.ErrEntityNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		fmt.Printf("ERROR: Failed to get entity tree: %v\n", err)

		http.Error(w, domain.ErrUnavailableServer.Error(), http.StatusInternalServerError)
		return
	}

	nodeTreeDTO := MapDomainToResponse(entityTree)

	// Serializa o DTO para JSON.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(nodeTreeDTO); err != nil {
		log.Printf("ERROR: Failed to write JSON response: %v", err)
	}
}

func (s *Server) GetEntityByProperty(w http.ResponseWriter, r *http.Request) {
	prop := r.PathValue("prop")
	if prop == "" {
		http.Error(w, "Query parameter 'prop' is required", http.StatusBadRequest)
		return
	}

	value := r.PathValue("value")
	if value == "" {
		http.Error(w, "Query parameter 'value' is required", http.StatusBadRequest)
		return
	}

	entityTree, err := s.graphService.GetTreeByEntityProperty(r.Context(), prop, value)
	if err != nil {
		if errors.Is(err, domain.ErrEntityNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		log.Printf("ERROR: Failed to get entity tree by property '%s - %s': %v\n", prop, value, err)

		http.Error(w, domain.ErrUnavailableServer.Error(), http.StatusInternalServerError)
		return
	}

	nodeTreeDTO := MapDomainToResponse(entityTree)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(nodeTreeDTO); err != nil {
		log.Printf("ERROR: Failed to write JSON response: %v", err)
	}
}
