package server

import (
	"context"
	_ "embed"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/iamkhattar/homelab/butler/internal/recovery"
)

type RecoveryAuthorizer interface {
	Authorize(context.Context, string) (string, error)
}

type RecoveryServer struct {
	mux     *http.ServeMux
	auth    RecoveryAuthorizer
	service *recovery.Service
}

//go:embed ui/recovery.html
var recoveryUI []byte

func NewRecovery(auth RecoveryAuthorizer, service *recovery.Service) *RecoveryServer {
	s := &RecoveryServer{mux: http.NewServeMux(), auth: auth, service: service}
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	s.mux.HandleFunc("GET /", s.handleUI)
	s.mux.Handle("GET /api/v1/recovery/status", s.protected(http.HandlerFunc(s.handleStatus)))
	s.mux.Handle("POST /api/v1/bootstrap/advance", s.protected(http.HandlerFunc(s.handleAdvance)))
	s.mux.Handle("PUT /api/v1/bootstrap/pocket-id-api-key", s.protected(http.HandlerFunc(s.handlePocketIDAPIKey)))
	s.mux.Handle("POST /api/v1/bootstrap/identity-verification", s.protected(http.HandlerFunc(s.handleIdentityVerification)))
	s.mux.Handle("GET /api/v1/bootstrap/certificate", s.protected(http.HandlerFunc(s.handleCertificateStatus)))
	s.mux.Handle("POST /api/v1/bootstrap/certificate/verify-dns", s.protected(http.HandlerFunc(s.handleVerifyDNS)))
	return s
}

func (s *RecoveryServer) handleCertificateStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.service.CertificateStatus(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *RecoveryServer) handleVerifyDNS(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	status, err := s.service.VerifyDNSDelegation(r.Context(), body.Confirm)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *RecoveryServer) handleIdentityVerification(w http.ResponseWriter, r *http.Request) {
	var evidence recovery.IdentityEvidence
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&evidence); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if err := s.service.ConfirmIdentity(r.Context(), evidence); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, s.service.Status(r.Context()))
}

func (s *RecoveryServer) handlePocketIDAPIKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		APIKey string `json:"apiKey"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if err := s.service.ImportPocketIDAPIKey(r.Context(), body.APIKey); err != nil {
		http.Error(w, "Pocket ID machine credential replacement failed", http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stored"})
}

func (s *RecoveryServer) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *RecoveryServer) protected(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		actor, err := s.auth.Authorize(r.Context(), token)
		if err != nil {
			slog.Warn("recovery authentication failed", "error", err)
			http.Error(w, "recovery authentication unavailable", http.StatusServiceUnavailable)
			return
		}
		if actor == "" {
			http.Error(w, "invalid recovery credential", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *RecoveryServer) handleUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(recoveryUI)
}

func (s *RecoveryServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.service.Status(r.Context()))
}

func (s *RecoveryServer) handleAdvance(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if err := s.service.Advance(r.Context(), body.Confirm); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, s.service.Status(r.Context()))
}
