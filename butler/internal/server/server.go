package server

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/iamkhattar/homelab/butler/internal/access"
	"github.com/iamkhattar/homelab/butler/internal/applications"
	"github.com/iamkhattar/homelab/butler/internal/identity"
	"github.com/iamkhattar/homelab/butler/internal/operations"
	"github.com/iamkhattar/homelab/butler/internal/pocketid"
	"github.com/iamkhattar/homelab/butler/internal/reconciler"
	"github.com/iamkhattar/homelab/butler/internal/vault"
)

// Server is the Butler HTTP server.
type Server struct {
	mux         *http.ServeMux
	scheduler   *reconciler.Scheduler
	vault       *vault.Client
	auth        *AuthMiddleware
	identity    *identity.Service
	apps        *applications.Store
	operations  *operations.Store
	credentials *access.CredentialService
}

//go:embed ui/index.html
var controlPlaneUI []byte

// New creates a new Server. If auth is nil, JWT protection is disabled.
func New(scheduler *reconciler.Scheduler, vc *vault.Client, auth *AuthMiddleware) *Server {
	s := &Server{
		mux:        http.NewServeMux(),
		scheduler:  scheduler,
		vault:      vc,
		auth:       auth,
		operations: operations.NewStore(250),
	}
	s.routes()
	return s
}

func (s *Server) ConfigureDomains(identityService *identity.Service, applicationStore *applications.Store, credentialService *access.CredentialService) {
	s.identity = identityService
	s.apps = applicationStore
	s.credentials = credentialService
}

func (s *Server) UseOperationsStore(store *operations.Store) {
	if store != nil {
		s.operations = store
	}
}

func (s *Server) routes() {
	// Unauthenticated.
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /readyz", s.handleReadyz)
	s.mux.HandleFunc("GET /", s.handleUI)
	if s.auth != nil {
		s.mux.HandleFunc("GET /auth/login", s.auth.Login)
		s.mux.HandleFunc("GET /auth/callback", s.auth.Callback)
		s.mux.HandleFunc("POST /auth/logout", s.auth.Logout)
	}
	// PKI CA chain is non-secret public PEM; safe to expose unauthenticated
	// so a future homelabctl trust command can fetch it before authentication.
	s.mux.HandleFunc("GET /api/v1/pki/ca-chain", s.handleCAChain)

	// Authenticated.
	s.mux.Handle("GET /api/v1/me", s.require(access.Viewer, http.HandlerFunc(s.handleMe)))
	s.mux.Handle("POST /api/v1/reconcile", s.require(access.Operator, http.HandlerFunc(s.handleReconcile)))
	s.mux.Handle("GET /api/v1/status", s.require(access.Viewer, http.HandlerFunc(s.handleStatus)))
	s.mux.Handle("GET /api/v1/operations", s.require(access.Viewer, http.HandlerFunc(s.handleOperations)))
	s.mux.Handle("GET /api/v1/events", s.require(access.Viewer, http.HandlerFunc(s.handleEvents)))
	s.mux.Handle("GET /api/v1/identity/users", s.require(access.Viewer, http.HandlerFunc(s.handleUsers)))
	s.mux.Handle("POST /api/v1/identity/users", s.require(access.Admin, http.HandlerFunc(s.handleUsers)))
	s.mux.Handle("PUT /api/v1/identity/users/{id}", s.require(access.Admin, http.HandlerFunc(s.handleUser)))
	s.mux.Handle("PUT /api/v1/identity/users/{id}/groups", s.require(access.Admin, http.HandlerFunc(s.handleUserGroups)))
	s.mux.Handle("GET /api/v1/identity/groups", s.require(access.Viewer, http.HandlerFunc(s.handleGroups)))
	s.mux.Handle("GET /api/v1/identity/clients", s.require(access.Viewer, http.HandlerFunc(s.handleClients)))
	s.mux.Handle("POST /api/v1/identity/clients/{id}/rotate", s.require(access.Admin, http.HandlerFunc(s.handleRotateClient)))
	s.mux.Handle("GET /api/v1/applications", s.require(access.Viewer, http.HandlerFunc(s.handleApplications)))
	s.mux.Handle("PUT /api/v1/applications/{name}", s.require(access.Admin, http.HandlerFunc(s.handleApplication)))
	s.mux.Handle("POST /api/v1/access/kubernetes-credentials", s.require(access.Admin, http.HandlerFunc(s.handleKubernetesCredential)))
}

func (s *Server) handleKubernetesCredential(w http.ResponseWriter, r *http.Request) {
	if s.credentials == nil {
		http.Error(w, "credential issuance unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Role string `json:"role"`
		TTL  string `json:"ttl"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	ttl, err := time.ParseDuration(body.TTL)
	if err != nil {
		http.Error(w, "invalid credential TTL", http.StatusBadRequest)
		return
	}
	principal, _ := access.PrincipalFrom(r.Context())
	credential, err := s.credentials.Issue(r.Context(), principal, body.Role, ttl)
	if err == nil {
		s.operations.Record("access.kubernetes-credential.issued", actorFrom(r), "Short-lived Kubernetes credential issued for approved role "+body.Role)
	}
	w.Header().Set("Cache-Control", "no-store")
	respondCode(w, http.StatusCreated, credential, err)
}

func (s *Server) handleUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(controlPlaneUI)
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

func (s *Server) require(role access.Role, next http.Handler) http.Handler {
	return s.protected(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := access.PrincipalFrom(r.Context())
		if s.auth == nil && !ok {
			principal = access.Principal{Subject: "local-test", Role: access.Admin}
			ok = true
		}
		if !ok || !access.Allows(principal.Role, role) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(access.WithPrincipal(r.Context(), principal)))
	}))
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	if s.vault.Token() == "" {
		http.Error(w, "vault not authenticated", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleReconcile(w http.ResponseWriter, r *http.Request) {
	actor := actorFrom(r)
	op := s.operations.Start(r.Context(), "reconcile", actor, s.scheduler.RunOnce)
	writeJSON(w, http.StatusAccepted, op)
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.scheduler.Statuses())
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	principal, _ := access.PrincipalFrom(r.Context())
	writeJSON(w, http.StatusOK, principal)
}

func (s *Server) handleOperations(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.operations.Operations())
}

func (s *Server) handleEvents(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.operations.Events())
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	if s.identity == nil {
		http.Error(w, "identity domain unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodGet {
		users, err := s.identity.ListUsers(r.Context())
		respond(w, users, err)
		return
	}
	var user pocketid.User
	if !decodeJSON(w, r, &user) {
		return
	}
	created, err := s.identity.CreateUser(r.Context(), user)
	if err == nil {
		s.operations.Record("identity.user.created", actorFrom(r), "Pocket ID user created: "+created.ID)
	}
	respondCode(w, http.StatusCreated, created, err)
}

func (s *Server) handleUser(w http.ResponseWriter, r *http.Request) {
	if s.identity == nil {
		http.Error(w, "identity domain unavailable", http.StatusServiceUnavailable)
		return
	}
	var user pocketid.User
	if !decodeJSON(w, r, &user) {
		return
	}
	updated, err := s.identity.UpdateUser(r.Context(), r.PathValue("id"), user)
	if err == nil {
		s.operations.Record("identity.user.updated", actorFrom(r), "Pocket ID user updated: "+r.PathValue("id"))
	}
	respond(w, updated, err)
}

func (s *Server) handleUserGroups(w http.ResponseWriter, r *http.Request) {
	if s.identity == nil {
		http.Error(w, "identity domain unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		GroupIDs []string `json:"groupIds"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	updated, err := s.identity.SetGroups(r.Context(), r.PathValue("id"), body.GroupIDs)
	if err == nil {
		s.operations.Record("identity.user.groups-updated", actorFrom(r), fmt.Sprintf("Pocket ID memberships updated for %s (%d groups)", r.PathValue("id"), len(body.GroupIDs)))
	}
	respond(w, updated, err)
}

func (s *Server) handleGroups(w http.ResponseWriter, r *http.Request) {
	if s.identity == nil {
		http.Error(w, "identity domain unavailable", http.StatusServiceUnavailable)
		return
	}
	groups, err := s.identity.ListGroups(r.Context())
	respond(w, groups, err)
}

func (s *Server) handleClients(w http.ResponseWriter, r *http.Request) {
	if s.identity == nil {
		http.Error(w, "identity domain unavailable", http.StatusServiceUnavailable)
		return
	}
	clients, err := s.identity.ListClients(r.Context())
	respond(w, clients, err)
}

func (s *Server) handleRotateClient(w http.ResponseWriter, r *http.Request) {
	if s.identity == nil {
		http.Error(w, "identity domain unavailable", http.StatusServiceUnavailable)
		return
	}
	err := s.identity.RotateClientSecret(r.Context(), r.PathValue("id"))
	if err == nil {
		s.operations.Record("identity.client.rotated", actorFrom(r), "Pocket ID client secret rotated and stored in Vault: "+r.PathValue("id"))
	}
	respond(w, map[string]string{"status": "rotated"}, err)
}

func (s *Server) handleApplications(w http.ResponseWriter, r *http.Request) {
	if s.apps == nil {
		http.Error(w, "applications domain unavailable", http.StatusServiceUnavailable)
		return
	}
	items, err := s.apps.List(r.Context())
	respond(w, items, err)
}

func (s *Server) handleApplication(w http.ResponseWriter, r *http.Request) {
	if s.apps == nil {
		http.Error(w, "applications domain unavailable", http.StatusServiceUnavailable)
		return
	}
	var item applications.Integration
	if !decodeJSON(w, r, &item) {
		return
	}
	if item.Name != "" && item.Name != r.PathValue("name") {
		http.Error(w, "application name must match the URL", http.StatusBadRequest)
		return
	}
	item.Name = r.PathValue("name")
	err := s.apps.Put(r.Context(), item)
	if err == nil {
		s.operations.Record("application.integration.updated", actorFrom(r), "Application integration updated: "+item.Name)
	}
	respond(w, item, err)
}

// handleCAChain serves the Vault PKI CA chain (root + intermediate, PEM
// encoded) so operators can fetch it via homelabctl or curl and trust
// the homelab root CA in their platform trust store.
//
// Returns 503 while butler is still bootstrapping Vault (no token yet) or
// while PKI hasn't been set up. Clients should treat 503 as "retry".
func (s *Server) handleCAChain(w http.ResponseWriter, r *http.Request) {
	if s.vault.Token() == "" {
		http.Error(w, "vault not authenticated yet, retry", http.StatusServiceUnavailable)
		return
	}
	chain, err := s.vault.CAChain(r.Context())
	if err != nil {
		slog.Warn("ca chain not yet available", "error", err)
		http.Error(w, "ca chain not yet available, retry", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(chain))
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encoding JSON response", "error", err)
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, out interface{}) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "request must contain exactly one JSON object", http.StatusBadRequest)
		return false
	}
	return true
}

func respond(w http.ResponseWriter, value interface{}, err error) {
	respondCode(w, http.StatusOK, value, err)
}

func respondCode(w http.ResponseWriter, code int, value interface{}, err error) {
	if err != nil {
		slog.Warn("control-plane request failed", "error", err)
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "must be") || strings.Contains(err.Error(), "invalid") {
			status = http.StatusBadRequest
		}
		http.Error(w, http.StatusText(status), status)
		return
	}
	writeJSON(w, code, value)
}

func actorFrom(r *http.Request) string {
	principal, ok := access.PrincipalFrom(r.Context())
	if !ok {
		return "unknown"
	}
	if principal.Email != "" {
		return principal.Email
	}
	return principal.Subject
}
