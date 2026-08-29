package control

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const LoginCallbackAddress = "127.0.0.1:17654"

type LoginOptions struct {
	Issuer   string
	ClientID string
	Timeout  time.Duration
	OpenURL  func(string) error
}

func InteractiveLogin(ctx context.Context, options LoginOptions) (Session, error) {
	if options.Issuer == "" || options.ClientID == "" {
		return Session{}, fmt.Errorf("Pocket ID issuer and client ID are required")
	}
	if options.Timeout <= 0 {
		options.Timeout = 3 * time.Minute
	}
	provider, err := oidc.NewProvider(ctx, options.Issuer)
	if err != nil {
		return Session{}, fmt.Errorf("discovering Pocket ID: %w", err)
	}
	listener, err := net.Listen("tcp", LoginCallbackAddress)
	if err != nil {
		return Session{}, fmt.Errorf("opening OIDC callback on %s: %w", LoginCallbackAddress, err)
	}
	defer listener.Close()

	state, err := randomURLToken(32)
	if err != nil {
		return Session{}, err
	}
	nonce, err := randomURLToken(32)
	if err != nil {
		return Session{}, err
	}
	verifier, err := randomURLToken(48)
	if err != nil {
		return Session{}, err
	}
	challengeBytes := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes[:])
	redirectURL := "http://" + LoginCallbackAddress + "/callback"
	oauthConfig := oauth2.Config{
		ClientID:    options.ClientID,
		Endpoint:    provider.Endpoint(),
		RedirectURL: redirectURL,
		Scopes:      []string{oidc.ScopeOpenID, "profile", "email", "groups"},
	}

	type result struct {
		session Session
		err     error
	}
	results := make(chan result, 1)
	mux := http.NewServeMux()
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	mux.HandleFunc("GET /callback", func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("state") != state {
			http.Error(w, "OIDC state did not match", http.StatusBadRequest)
			results <- result{err: fmt.Errorf("OIDC state did not match")}
			return
		}
		if providerError := request.URL.Query().Get("error"); providerError != "" {
			http.Error(w, "Pocket ID rejected login", http.StatusUnauthorized)
			results <- result{err: fmt.Errorf("Pocket ID rejected login: %s", providerError)}
			return
		}
		token, exchangeErr := oauthConfig.Exchange(request.Context(), request.URL.Query().Get("code"), oauth2.VerifierOption(verifier))
		if exchangeErr != nil {
			http.Error(w, "OIDC token exchange failed", http.StatusBadGateway)
			results <- result{err: fmt.Errorf("exchanging OIDC code: %w", exchangeErr)}
			return
		}
		rawIDToken, ok := token.Extra("id_token").(string)
		if !ok || rawIDToken == "" {
			http.Error(w, "Pocket ID omitted the ID token", http.StatusBadGateway)
			results <- result{err: fmt.Errorf("Pocket ID omitted the ID token")}
			return
		}
		verified, verifyErr := provider.Verifier(&oidc.Config{ClientID: options.ClientID}).Verify(request.Context(), rawIDToken)
		if verifyErr != nil {
			http.Error(w, "OIDC token verification failed", http.StatusUnauthorized)
			results <- result{err: fmt.Errorf("verifying Pocket ID token: %w", verifyErr)}
			return
		}
		var claims struct {
			Nonce string `json:"nonce"`
		}
		if claimErr := verified.Claims(&claims); claimErr != nil || claims.Nonce != nonce {
			http.Error(w, "OIDC nonce did not match", http.StatusUnauthorized)
			results <- result{err: fmt.Errorf("OIDC nonce did not match")}
			return
		}
		_, _ = w.Write([]byte("Login complete. You can close this window and return to homelabctl."))
		results <- result{session: Session{Issuer: options.Issuer, ClientID: options.ClientID, IDToken: rawIDToken, ExpiresAt: verified.Expiry}}
	})
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			select {
			case results <- result{err: serveErr}:
			default:
			}
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	authURL := oauthConfig.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	if options.OpenURL != nil {
		if err := options.OpenURL(authURL); err != nil {
			return Session{}, fmt.Errorf("opening browser: %w", err)
		}
	}
	waitCtx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()
	select {
	case completed := <-results:
		return completed.session, completed.err
	case <-waitCtx.Done():
		return Session{}, fmt.Errorf("waiting for Pocket ID login: %w", waitCtx.Err())
	}
}

func randomURLToken(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating OIDC nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
