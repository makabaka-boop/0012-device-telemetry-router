package httpapi

import (
	"net/http"
	"strconv"
)

func (s *Server) handleListDeliveries(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("event_id")
	deliveries, err := s.svc.ListDeliveries(r.Context(), eventID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": deliveries})
}

func (s *Server) handleReplayEvent(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("event_id")
	res, err := s.svc.ReplayEvent(operatorContext(r), eventID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := s.svc.ListAudit(r.Context(), limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
