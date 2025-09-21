package http

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"userprofilepoc/src/domain"
)

func (s *Server) IngestTemporalData(w http.ResponseWriter, r *http.Request) {
	var request domain.SyncTemporalPropertyRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	err = s.temporalDataService.UpsertDataPoints(r.Context(), request)
	if err != nil {
		log.Printf("ERROR: Failed to ingest temporal data: %v", err)
		http.Error(w, domain.ErrUnavailableServer.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintln(w, `{"status": "ingestion request accepted for processing"}`)
}
