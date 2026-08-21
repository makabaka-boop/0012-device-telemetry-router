package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"device-telemetry-router/internal/service"
)

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("size"))

	var from, to *time.Time
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = &t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = &t
		}
	}

	pageResult, err := s.svc.QueryEvents(r.Context(), service.EventQuery{
		DeviceID: q.Get("device_id"),
		Metric:   q.Get("metric"),
		From:     from,
		To:       to,
		Status:   q.Get("status"),
		Page:     page,
		Size:     size,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pageResult)
}

func (s *Server) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	e, err := s.svc.GetEvent(r.Context(), r.PathValue("event_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.svc.Stats(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
