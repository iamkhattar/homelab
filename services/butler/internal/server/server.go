package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/iamkhattar/homelab/services/butler/internal/reconciler"
	"github.com/iamkhattar/homelab/services/butler/internal/vault"
)

// Server is the Butler HTTP server.
type Server struct {
	mux       *http.ServeMux
	scheduler *reconciler.Scheduler
	vault     *vault.Client
	auth      *AuthMiddleware
}

// New creates a new Server. If auth is nil, JWT protection is disabled.
func New(scheduler *reconciler.Scheduler, vc *vault.Client, auth *AuthMiddleware) *Server {
	s := &Server{
		mux:       http.NewServeMux(),
		scheduler: scheduler,
		vault:     vc,
		auth:      auth,
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	// Unauthenticated.
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /readyz", s.handleReadyz)

	// Authenticated.
	s.mux.Handle("POST /api/v1/reconcile", s.protected(http.HandlerFunc(s.handleReconcile)))
	s.mux.Handle("GET /api/v1/status", s.protected(http.HandlerFunc(s.handleStatus)))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) protected(next http.Handler) http.Handler {
	if s.auth == nil {
		return next
	}
	return s.auth.Wrap(next)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	if s.vault.Token() == "" {
		http.Error(w, "vault not authenticated", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) handleReconcile(w http.ResponseWriter, r *http.Request) {
	if err := s.scheduler.RunOnce(r.Context()); err != nil {
		slog.Error("manual reconciliation failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.scheduler.Statuses())
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
