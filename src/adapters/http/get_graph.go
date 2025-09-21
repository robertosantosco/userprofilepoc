package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
	"userprofilepoc/src/domain"
)

func (s *Server) GetGraphByID(w http.ResponseWriter, r *http.Request) {
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

	depthLimitStr := r.URL.Query().Get("depthLimit")
	depthLimit := 5 // Default value
	if depthLimitStr != "" {
		var err error
		depthLimit, err = strconv.Atoi(depthLimitStr)
		if err != nil {
			http.Error(w, "Invalid depthLimit format", http.StatusBadRequest)
			return
		}
	}

	startTimeStr := r.URL.Query().Get("startTime")
	var startTime time.Time
	if startTimeStr == "" {
		now := time.Now()
		startTime = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	} else {
		var err error
		startTime, err = time.Parse("2006-01-02", startTimeStr)
		if err != nil {
			http.Error(w, "Invalid startTime format. Use 'YYYY-MM-DD'", http.StatusBadRequest)
			return
		}
		startTime = time.Date(startTime.Year(), startTime.Month(), 1, 0, 0, 0, 0, startTime.Location())
	}

	now := time.Now()
	if startTime.After(now) {
		http.Error(w, "startTime cannot be in the future", http.StatusBadRequest)
		return
	}

	if startTime.Before(now.AddDate(-1, 0, 0)) {
		http.Error(w, "startTime cannot be older than 12 months", http.StatusBadRequest)
		return
	}

	entityTree, err := s.graphService.GetTreeByEntityID(r.Context(), entityID, depthLimit, startTime)
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

func (s *Server) GetGraphByProperty(w http.ResponseWriter, r *http.Request) {
	prop := r.PathValue("prop")
	if prop == "" {
		http.Error(w, "prop is required", http.StatusBadRequest)
		return
	}

	value := r.PathValue("value")
	if value == "" {
		http.Error(w, "value is required", http.StatusBadRequest)
		return
	}

	depthLimitStr := r.URL.Query().Get("depthLimit")
	depthLimit := 5 // Default value
	if depthLimitStr != "" {
		var err error
		depthLimit, err = strconv.Atoi(depthLimitStr)
		if err != nil {
			http.Error(w, "Invalid depthLimit format", http.StatusBadRequest)
			return
		}
	}

	startTimeStr := r.URL.Query().Get("startTime")
	var startTime time.Time
	if startTimeStr == "" {
		now := time.Now()
		startTime = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	} else {
		var err error
		startTime, err = time.Parse("2006-01-02", startTimeStr)
		if err != nil {
			http.Error(w, "Invalid startTime format. Use 'YYYY-MM-DD'", http.StatusBadRequest)
			return
		}
		startTime = time.Date(startTime.Year(), startTime.Month(), 1, 0, 0, 0, 0, startTime.Location())
	}

	now := time.Now()
	if startTime.After(now) {
		http.Error(w, "startTime cannot be in the future", http.StatusBadRequest)
		return
	}

	if startTime.Before(now.AddDate(-1, 0, 0)) {
		http.Error(w, "startTime cannot be older than 12 months", http.StatusBadRequest)
		return
	}

	entityTree, err := s.graphService.GetTreeByEntityProperty(r.Context(), prop, value, depthLimit, startTime)
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
