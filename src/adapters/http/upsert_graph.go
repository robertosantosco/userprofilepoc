package http

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"userprofilepoc/src/domain"
)

func (s *Server) SyncGraph(w http.ResponseWriter, r *http.Request) {
	var request domain.SyncGraphRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	err = s.graphService.SyncGraph(r.Context(), request)
	if err != nil {
		log.Printf("ERROR: Failed to sync graph: %v", err)
		http.Error(w, domain.ErrUnavailableServer.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintln(w, `{"status": "sync request accepted for processing"}`)
}
