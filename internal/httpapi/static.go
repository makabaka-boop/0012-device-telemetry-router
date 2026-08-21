package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
)

// staticDir is the root directory containing the compiled frontend assets.
// It defaults to internal/httpapi/assets and can be overridden with the
// STATIC_DIR environment variable for container deployments.
func staticDir() string {
	if d := os.Getenv("STATIC_DIR"); d != "" {
		return d
	}
	return "internal/httpapi/assets"
}

// handleAssets serves CSS/JS files under /assets/.
func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path[len("/assets/"):]
	p := filepath.Join(staticDir(), filepath.Clean(name))
	http.ServeFile(w, r, p)
}

// handleIndex serves the single-page frontend at the root and any page
// route that is not under /api/ or /assets/.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/devices" && r.URL.Path != "/telemetry" &&
		r.URL.Path != "/rules" && r.URL.Path != "/events" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(staticDir(), "index.html"))
}
