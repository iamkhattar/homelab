package control

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

type VaultOIDCOptions struct {
	Address       string
	Role          string
	Mount         string
	ListenAddress string
	CallbackHost  string
	Timeout       time.Duration
	OpenURL       func(string) error
	HTTPClient    *http.Client
}

type VaultIdentityEvidence struct {
	Policies []string
}

type vaultSecret struct {
	Data map[string]interface{} `json:"data"`
	Auth *struct {
		ClientToken string            `json:"client_token"`
		Policies    []string          `json:"policies"`
		Metadata    map[string]string `json:"metadata"`
	} `json:"auth"`
	Errors []string `json:"errors"`
}

// VerifyVaultOIDCLogin completes Vault's browser OIDC API flow, validates the
// resulting token with lookup-self, and revokes it before returning non-secret
// policy evidence. It never writes the Vault token to disk or Butler.
func VerifyVaultOIDCLogin(ctx context.Context, options VaultOIDCOptions) (VaultIdentityEvidence, error) {
	if options.Address == "" || options.Role == "" {
		return VaultIdentityEvidence{}, fmt.Errorf("Vault address and OIDC role are required")
	}
	if options.Mount == "" {
		options.Mount = "jwt"
	}
	if options.ListenAddress == "" {
		options.ListenAddress = "127.0.0.1:8250"
	}
	if options.CallbackHost == "" {
		options.CallbackHost = "localhost"
	}
	if options.Timeout <= 0 {
		options.Timeout = 3 * time.Minute
	}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	listener, err := net.Listen("tcp", options.ListenAddress)
	if err != nil {
		return VaultIdentityEvidence{}, fmt.Errorf("opening Vault OIDC callback: %w", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://%s:%d/oidc/callback", options.CallbackHost, port)
	clientNonce, err := randomVaultNonce(20)
	if err != nil {
		return VaultIdentityEvidence{}, err
	}
	authURL, err := vaultAuthURL(ctx, options, redirectURI, clientNonce)
	if err != nil {
		return VaultIdentityEvidence{}, err
	}

	type result struct {
		secret vaultSecret
		err    error
	}
	results := make(chan result, 1)
	mux := http.NewServeMux()
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	mux.HandleFunc("/oidc/callback", func(w http.ResponseWriter, request *http.Request) {
		values := url.Values{
			"state":        {request.FormValue("state")},
			"code":         {request.FormValue("code")},
			"id_token":     {request.FormValue("id_token")},
			"client_nonce": {clientNonce},
		}
		var secret vaultSecret
		err := vaultJSON(ctx, options.HTTPClient, http.MethodGet, strings.TrimRight(options.Address, "/")+"/v1/auth/"+url.PathEscape(options.Mount)+"/oidc/callback?"+values.Encode(), "", nil, &secret)
		if err != nil {
			http.Error(w, "Vault OIDC verification failed", http.StatusUnauthorized)
		} else {
			_, _ = w.Write([]byte("Vault login verified. You can close this window and return to homelabctl."))
		}
		results <- result{secret: secret, err: err}
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
	if options.OpenURL != nil {
		if err := options.OpenURL(authURL); err != nil {
			return VaultIdentityEvidence{}, fmt.Errorf("opening Vault OIDC login: %w", err)
		}
	}
	waitCtx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()
	select {
	case completed := <-results:
		if completed.err != nil {
			return VaultIdentityEvidence{}, completed.err
		}
		return verifyAndRevokeVaultToken(waitCtx, options, completed.secret)
	case <-waitCtx.Done():
		return VaultIdentityEvidence{}, fmt.Errorf("waiting for Vault OIDC login: %w", waitCtx.Err())
	}
}

func vaultAuthURL(ctx context.Context, options VaultOIDCOptions, redirectURI, nonce string) (string, error) {
	body := map[string]string{"role": options.Role, "redirect_uri": redirectURI, "client_nonce": nonce}
	var response vaultSecret
	endpoint := strings.TrimRight(options.Address, "/") + "/v1/auth/" + url.PathEscape(options.Mount) + "/oidc/auth_url"
	if err := vaultJSON(ctx, options.HTTPClient, http.MethodPost, endpoint, "", body, &response); err != nil {
		return "", fmt.Errorf("requesting Vault OIDC authorization URL: %w", err)
	}
	authURL, _ := response.Data["auth_url"].(string)
	if authURL == "" {
		return "", fmt.Errorf("Vault returned no OIDC authorization URL for role %q", options.Role)
	}
	return authURL, nil
}

func verifyAndRevokeVaultToken(ctx context.Context, options VaultOIDCOptions, login vaultSecret) (VaultIdentityEvidence, error) {
	if login.Auth == nil || login.Auth.ClientToken == "" {
		return VaultIdentityEvidence{}, fmt.Errorf("Vault OIDC callback returned no client token")
	}
	token := login.Auth.ClientToken
	revokeURL := strings.TrimRight(options.Address, "/") + "/v1/auth/token/revoke-self"
	defer func() {
		revokeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = vaultJSON(revokeCtx, options.HTTPClient, http.MethodPost, revokeURL, token, struct{}{}, nil)
	}()
	var lookup vaultSecret
	lookupURL := strings.TrimRight(options.Address, "/") + "/v1/auth/token/lookup-self"
	if err := vaultJSON(ctx, options.HTTPClient, http.MethodGet, lookupURL, token, nil, &lookup); err != nil {
		return VaultIdentityEvidence{}, fmt.Errorf("verifying Vault OIDC token: %w", err)
	}
	policies := append([]string(nil), login.Auth.Policies...)
	if raw, ok := lookup.Data["policies"].([]interface{}); ok {
		for _, item := range raw {
			if policy, ok := item.(string); ok && !slices.Contains(policies, policy) {
				policies = append(policies, policy)
			}
		}
	}
	if !slices.Contains(policies, "vault-admin") || !slices.Contains(policies, "k8s-admin") {
		return VaultIdentityEvidence{}, fmt.Errorf("Vault OIDC role omitted required policies")
	}
	return VaultIdentityEvidence{Policies: policies}, nil
}

func vaultJSON(ctx context.Context, client *http.Client, method, endpoint, token string, input, output interface{}) error {
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("X-Vault-Token", token)
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
		return fmt.Errorf("Vault returned %s: %s", response.Status, strings.TrimSpace(string(raw)))
	}
	if output == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(output)
}

func randomVaultNonce(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating Vault OIDC nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
