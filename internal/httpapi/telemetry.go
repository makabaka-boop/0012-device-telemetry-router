package httpapi

import (
	"net/http"

	"device-telemetry-router/internal/domain"
)

type telemetryReq struct {
	RawText string `json:"raw_text"`
}

func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	var req telemetryReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, domain.ErrInvalidField.WithDetails(map[string]any{"cause": err.Error()}))
		return
	}
	if req.RawText == "" {
		writeError(w, domain.ErrInvalidField.WithDetails(map[string]any{"field": "raw_text"}))
		return
	}
	res, err := s.svc.IngestTelemetry(operatorContext(r), req.RawText)
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusCreated
	if res.Duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, res)
}
