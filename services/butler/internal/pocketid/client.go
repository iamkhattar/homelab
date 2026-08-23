// Package pocketid is a minimal client for the Pocket-ID admin API.
//
// Pocket-ID exposes a REST surface under /api for managing OIDC clients,
// users, and groups. Butler uses this client to provision the OAuth clients
// declared in its ConfigMap (the OAuthClients reconciler).
//
// Authentication is via an admin API key minted in the Pocket-ID UI by a
// human operator and persisted in Vault at secret/pocket-id/admin-api-key.
// The reconciler fetches it from Vault and passes it here on construction.
package pocketid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrAPIKeyMissing signals to callers that no admin API key is available
// yet, so they can degrade gracefully (e.g. fall back to staging-Secret
// ingestion mode).
type ErrAPIKeyMissing struct{}

func (ErrAPIKeyMissing) Error() string {
	return "pocket-id admin api key not found; provision one in the Pocket-ID UI and store it at secret/pocket-id/admin-api-key"
}

// Client is a tiny HTTP client for Pocket-ID's admin API.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewClient builds a Pocket-ID admin API client. baseURL is the externally-
// reachable URL of Pocket-ID (e.g. https://auth.shivlab.com). apiKey is the
// admin API token. If apiKey is empty, all methods return ErrAPIKeyMissing.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// OIDCClient is the subset of the Pocket-ID client representation butler
// cares about. Field names match the Pocket-ID REST API.
type OIDCClient struct {
	ID           string   `json:"id,omitempty"`
	Name         string   `json:"name"`
	IsPublic     bool     `json:"isPublic"`
	CallbackURLs []string `json:"callbackUrls"`
	LogoutURLs   []string `json:"logoutCallbackUrls,omitempty"`
	// Returned only on create.
	Secret string `json:"secret,omitempty"`
}

// ListClients returns all OIDC clients configured in Pocket-ID.
func (c *Client) ListClients(ctx context.Context) ([]OIDCClient, error) {
	if c.apiKey == "" {
		return nil, ErrAPIKeyMissing{}
	}
	var clients []OIDCClient
	if err := c.do(ctx, http.MethodGet, "/api/oidc/clients", nil, &clients); err != nil {
		return nil, fmt.Errorf("listing oidc clients: %w", err)
	}
	return clients, nil
}

// CreateClient creates a new OIDC client and returns the populated client
// (including the generated secret). The caller is responsible for
// persisting the secret immediately — Pocket-ID won't surface it again on
// subsequent reads.
func (c *Client) CreateClient(ctx context.Context, in OIDCClient) (*OIDCClient, error) {
	if c.apiKey == "" {
		return nil, ErrAPIKeyMissing{}
	}
	var out OIDCClient
	if err := c.do(ctx, http.MethodPost, "/api/oidc/clients", in, &out); err != nil {
		return nil, fmt.Errorf("creating oidc client %q: %w", in.Name, err)
	}
	return &out, nil
}

// UpdateClient updates an existing OIDC client (callback URLs, name, etc.).
// The secret is unchanged by this call.
func (c *Client) UpdateClient(ctx context.Context, id string, in OIDCClient) error {
	if c.apiKey == "" {
		return ErrAPIKeyMissing{}
	}
	if err := c.do(ctx, http.MethodPut, "/api/oidc/clients/"+id, in, nil); err != nil {
		return fmt.Errorf("updating oidc client %q: %w", id, err)
	}
	return nil
}

// RotateSecret asks Pocket-ID to generate a fresh secret for the client and
// returns the new value.
func (c *Client) RotateSecret(ctx context.Context, id string) (string, error) {
	if c.apiKey == "" {
		return "", ErrAPIKeyMissing{}
	}
	var resp struct {
		Secret string `json:"secret"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/oidc/clients/"+id+"/secret", nil, &resp); err != nil {
		return "", fmt.Errorf("rotating secret for oidc client %q: %w", id, err)
	}
	return resp.Secret, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out interface{}) error {
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
		reqBody = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	// Pocket-ID uses an X-API-KEY header for admin auth. Some deployments
	// also accept Authorization: Bearer; sending both is harmless and lets
	// us work across versions.
	req.Header.Set("X-API-KEY", c.apiKey)
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("http %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("http %s %s: status %d: %s", method, path, resp.StatusCode, string(raw))
	}

	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			// Empty bodies are fine; only fail if there's something we
			// couldn't parse.
			if err != io.EOF {
				return fmt.Errorf("decoding response: %w", err)
			}
		}
	}
	return nil
}
