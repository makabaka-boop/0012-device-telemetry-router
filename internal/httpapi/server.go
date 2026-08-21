// Package httpapi implements the HTTP transport: routing, middleware and
// resource handlers.
package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"device-telemetry-router/internal/auth"
	"device-telemetry-router/internal/domain"
	"device-telemetry-router/internal/service"
)

// Server wires handlers to the application service.
type Server struct {
	svc     *service.Service
	handler http.Handler
}

// ServerDeps carries optional cross-cutting dependencies used to build the
// HTTP handler tree.
type ServerDeps struct {
	Metrics http.Handler // optional Prometheus metrics handler (nil disables)
	Auth    *auth.Auth   // optional API key authenticator (nil disables)
}

// NewServer builds the HTTP handler tree.
func NewServer(svc *service.Service) *Server {
	return NewServerWith(svc, ServerDeps{})
}

// NewServerWith builds the handler tree with optional metrics and auth.
func NewServerWith(svc *service.Service, deps ServerDeps) *Server {
	s := &Server{svc: svc}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealthz)
	if deps.Metrics != nil {
		mux.Handle("GET /metrics", deps.Metrics)
	}
	mux.HandleFunc("POST /api/v1/devices", s.handleCreateDevice)
	mux.HandleFunc("GET /api/v1/devices", s.handleListDevices)
	mux.HandleFunc("GET /api/v1/devices/{device_id}", s.handleGetDevice)
	mux.HandleFunc("POST /api/v1/devices/{device_id}/status", s.handleDeviceStatus)
	mux.HandleFunc("POST /api/v1/telemetry", s.handleTelemetry)
	mux.HandleFunc("POST /api/v1/rules", s.handleCreateRule)
	mux.HandleFunc("GET /api/v1/rules", s.handleListRules)
	mux.HandleFunc("GET /api/v1/rules/{rule_id}", s.handleGetRule)
	mux.HandleFunc("PUT /api/v1/rules/{rule_id}", s.handleUpdateRule)
	mux.HandleFunc("DELETE /api/v1/rules/{rule_id}", s.handleDeleteRule)
	mux.HandleFunc("GET /api/v1/rules/{rule_id}/changes", s.handleRuleChanges)
	mux.HandleFunc("GET /api/v1/events", s.handleListEvents)
	mux.HandleFunc("GET /api/v1/events/{event_id}", s.handleGetEvent)
	mux.HandleFunc("GET /api/v1/events/{event_id}/deliveries", s.handleListDeliveries)
	mux.HandleFunc("POST /api/v1/events/{event_id}/replay", s.handleReplayEvent)
	mux.HandleFunc("GET /api/v1/stats", s.handleStats)
	mux.HandleFunc("GET /api/v1/audit", s.handleListAudit)

	// Static frontend (page routes and assets).
	mux.HandleFunc("GET /assets/", s.handleAssets)
	mux.HandleFunc("GET /", s.handleIndex)

	var h http.Handler = mux
	if deps.Auth != nil {
		h = s.wrapAuth(deps.Auth, h)
	}
	s.handler = withRecovery(withLogging(h))
	return s
}

// Handler returns the root http.Handler.
func (s *Server) Handler() http.Handler {
	return s.handler
}

// wrapAuth enforces API key authentication on mutating API routes while
// leaving health, metrics, static assets and read endpoints publicly
// reachable. Auth (and thus the 401 semantic) is enabled only when an
// authenticator is wired, keeping the initial deployment open.
func (s *Server) wrapAuth(a *auth.Auth, next http.Handler) http.Handler {
	fn := auth.Middleware(a)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requiresAuth(r) {
			fn(next).ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requiresAuth reports whether the request mutates API state and therefore
// needs an authenticated operator.
func requiresAuth(r *http.Request) bool {
	if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodDelete {
		return false
	}
	p := r.URL.Path
	return len(p) >= 8 && p[:8] == "/api/v1/"
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	var de *domain.Error
	if errors.As(err, &de) {
		if de.Status == 200 {
			de.Status = 500
		}
		writeJSON(w, de.Status, map[string]any{
			"error": map[string]any{
				"code":    de.Code,
				"message": de.Message,
				"details": de.Details,
			},
		})
		return
	}
	log.Printf("internal error: %v", err)
	writeJSON(w, http.StatusInternalServerError, map[string]any{
		"error": map[string]any{
			"code":    "INTERNAL_ERROR",
			"message": "internal server error",
		},
	})
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// withLogging logs each request.
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

// withRecovery recovers from panics and returns a 500.
func withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v", rec)
				writeJSON(w, http.StatusInternalServerError, map[string]any{
					"error": map[string]any{
						"code":    "INTERNAL_ERROR",
						"message": "internal server error",
					},
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
