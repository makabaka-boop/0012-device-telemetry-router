package httpapi

import (
	"crypto/rand"
	"math/big"
	"net/http"

	"device-telemetry-router/internal/domain"
)

type createDeviceReq struct {
	DeviceID        string         `json:"device_id"`
	Name            string         `json:"name"`
	ProtocolVersion string         `json:"protocol_version"`
	Metadata        map[string]any `json:"metadata"`
}

func (s *Server) handleCreateDevice(w http.ResponseWriter, r *http.Request) {
	var req createDeviceReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, domain.ErrInvalidField.WithDetails(map[string]any{"cause": err.Error()}))
		return
	}
	if req.DeviceID == "" {
		req.DeviceID = newDeviceID()
	}
	d, err := s.svc.RegisterDevice(operatorContext(r), req.DeviceID, req.Name, req.ProtocolVersion, req.Metadata)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.svc.ListDevices(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": devices})
}

func (s *Server) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("device_id")
	d, err := s.svc.GetDevice(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

type deviceStatusReq struct {
	Status string `json:"status"`
}

func (s *Server) handleDeviceStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("device_id")
	var req deviceStatusReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, domain.ErrInvalidField.WithDetails(map[string]any{"cause": err.Error()}))
		return
	}
	if err := s.svc.SetDeviceStatus(operatorContext(r), id, domain.DeviceStatus(req.Status)); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"device_id": id, "status": req.Status})
}

func newDeviceID() string {
	// Generate a random uppercase alphanumeric id of length 12.
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 12)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			b[i] = 'A'
			continue
		}
		b[i] = alphabet[n.Int64()]
	}
	return string(b)
}
