// Package pocketid is a minimal client for the Pocket-ID admin API.
//
// Pocket-ID exposes a REST surface under /api for managing OIDC clients,
// users, and groups. Butler uses this client to provision the OIDC clients
// declared by PocketIDClient resources.
//
// Authentication is via a declaratively generated static machine credential.
// Butler stores it in Vault, VSO projects it to Pocket ID, and reconcilers read
// the same value directly from Vault. It is never returned through an API.
package pocketid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	ManagementCredentialVaultPath = "security/pocket-id"
	ManagementCredentialField     = "static-api-key"
)

// ErrAPIKeyMissing signals to callers that the Vault-backed machine credential
// has not converged yet, so reconciliation can report a waiting condition.
type ErrAPIKeyMissing struct{}

func (ErrAPIKeyMissing) Error() string {
	return "Pocket ID machine credential is not available in Vault"
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
			Timeout:   15 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

// OIDCClient is the subset of the Pocket-ID client representation butler
// cares about. Field names match the Pocket-ID REST API.
type OIDCClient struct {
	ID           string   `json:"id,omitempty"`
	Name         string   `json:"name"`
	IsPublic     bool     `json:"isPublic"`
	PKCEEnabled  bool     `json:"pkceEnabled"`
	CallbackURLs []string `json:"callbackURLs"`
	LogoutURLs   []string `json:"logoutCallbackURLs,omitempty"`
	// Returned only on create.
	Secret   string `json:"secret,omitempty"`
	SecretID string `json:"-"`
}

type OIDCClientSecret struct {
	ID       string `json:"id"`
	Secret   string `json:"secret,omitempty"`
	IsActive bool   `json:"isActive,omitempty"`
}

// ListClients returns all OIDC clients configured in Pocket-ID.
func (c *Client) ListClients(ctx context.Context) ([]OIDCClient, error) {
	if c.apiKey == "" {
		return nil, ErrAPIKeyMissing{}
	}
	var page paginated[OIDCClient]
	if err := c.do(ctx, http.MethodGet, "/api/oidc/clients?pagination[limit]=100", nil, &page); err != nil {
		return nil, fmt.Errorf("listing oidc clients: %w", err)
	}
	return page.Data, nil
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
	if !in.IsPublic {
		secret, err := c.CreateSecret(ctx, out.ID)
		if err != nil {
			return nil, err
		}
		out.Secret = secret.Secret
		out.SecretID = secret.ID
	}
	return &out, nil
}

// UpdateClient updates an existing OIDC client (callback URLs, name, etc.).
// The secret is unchanged by this call.
func (c *Client) UpdateClient(ctx context.Context, id string, in OIDCClient) error {
	if c.apiKey == "" {
		return ErrAPIKeyMissing{}
	}
	if err := c.do(ctx, http.MethodPut, "/api/oidc/clients/"+url.PathEscape(id), in, nil); err != nil {
		return fmt.Errorf("updating oidc client %q: %w", id, err)
	}
	return nil
}

// CreateSecret adds a new secret to a confidential OIDC client. Pocket ID
// returns its value exactly once, so callers must persist it immediately.
func (c *Client) CreateSecret(ctx context.Context, id string) (OIDCClientSecret, error) {
	if c.apiKey == "" {
		return OIDCClientSecret{}, ErrAPIKeyMissing{}
	}
	var resp OIDCClientSecret
	if err := c.do(ctx, http.MethodPost, "/api/oidc/clients/"+url.PathEscape(id)+"/secrets", struct{}{}, &resp); err != nil {
		return OIDCClientSecret{}, fmt.Errorf("creating secret for oidc client %q: %w", id, err)
	}
	if resp.ID == "" || resp.Secret == "" {
		return OIDCClientSecret{}, fmt.Errorf("Pocket ID returned an incomplete secret for oidc client %q", id)
	}
	return resp, nil
}

func (c *Client) ListClientSecrets(ctx context.Context, id string) ([]OIDCClientSecret, error) {
	if c.apiKey == "" {
		return nil, ErrAPIKeyMissing{}
	}
	var out []OIDCClientSecret
	if err := c.do(ctx, http.MethodGet, "/api/oidc/clients/"+url.PathEscape(id)+"/secrets", nil, &out); err != nil {
		return nil, fmt.Errorf("listing secrets for oidc client %q: %w", id, err)
	}
	return out, nil
}

func (c *Client) DeleteClientSecret(ctx context.Context, clientID, secretID string) error {
	if c.apiKey == "" {
		return ErrAPIKeyMissing{}
	}
	if err := c.do(ctx, http.MethodDelete, "/api/oidc/clients/"+url.PathEscape(clientID)+"/secrets/"+url.PathEscape(secretID), nil, nil); err != nil {
		return fmt.Errorf("deleting secret %q for oidc client %q: %w", secretID, clientID, err)
	}
	return nil
}

// UserGroup is the API representation Butler needs for declarative groups.
type UserGroup struct {
	ID           string `json:"id,omitempty"`
	Name         string `json:"name"`
	FriendlyName string `json:"friendlyName"`
}

// User is the stable subset of Pocket ID v2's user representation exposed by
// Butler. Passkeys and generated credentials are intentionally excluded.
type User struct {
	ID            string      `json:"id,omitempty"`
	Username      string      `json:"username"`
	Email         *string     `json:"email,omitempty"`
	EmailVerified bool        `json:"emailVerified"`
	FirstName     string      `json:"firstName,omitempty"`
	LastName      string      `json:"lastName,omitempty"`
	DisplayName   string      `json:"displayName,omitempty"`
	IsAdmin       bool        `json:"isAdmin"`
	Disabled      bool        `json:"disabled"`
	UserGroups    []UserGroup `json:"userGroups,omitempty"`
	UserGroupIDs  []string    `json:"userGroupIds,omitempty"`
}

func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	if c.apiKey == "" {
		return nil, ErrAPIKeyMissing{}
	}
	var page paginated[User]
	if err := c.do(ctx, http.MethodGet, "/api/users?pagination[limit]=100", nil, &page); err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	return page.Data, nil
}

func (c *Client) CreateUser(ctx context.Context, user User) (*User, error) {
	if c.apiKey == "" {
		return nil, ErrAPIKeyMissing{}
	}
	var out User
	if err := c.do(ctx, http.MethodPost, "/api/users", user, &out); err != nil {
		return nil, fmt.Errorf("creating user %q: %w", user.Username, err)
	}
	return &out, nil
}

func (c *Client) UpdateUser(ctx context.Context, id string, user User) (*User, error) {
	if c.apiKey == "" {
		return nil, ErrAPIKeyMissing{}
	}
	var out User
	if err := c.do(ctx, http.MethodPut, "/api/users/"+url.PathEscape(id), user, &out); err != nil {
		return nil, fmt.Errorf("updating user %q: %w", id, err)
	}
	return &out, nil
}

func (c *Client) UpdateUserGroups(ctx context.Context, id string, groupIDs []string) (*User, error) {
	if c.apiKey == "" {
		return nil, ErrAPIKeyMissing{}
	}
	var out User
	body := struct {
		UserGroupIDs []string `json:"userGroupIds"`
	}{UserGroupIDs: groupIDs}
	if err := c.do(ctx, http.MethodPut, "/api/users/"+url.PathEscape(id)+"/user-groups", body, &out); err != nil {
		return nil, fmt.Errorf("updating groups for user %q: %w", id, err)
	}
	return &out, nil
}

func (c *Client) DeleteUser(ctx context.Context, id string) error {
	if c.apiKey == "" {
		return ErrAPIKeyMissing{}
	}
	if err := c.do(ctx, http.MethodDelete, "/api/users/"+url.PathEscape(id), nil, nil); err != nil {
		return fmt.Errorf("deleting user %q: %w", id, err)
	}
	return nil
}

// ListUserGroups lists the first 100 groups, which is intentionally well
// above the bounded homelab configuration.
func (c *Client) ListUserGroups(ctx context.Context) ([]UserGroup, error) {
	if c.apiKey == "" {
		return nil, ErrAPIKeyMissing{}
	}
	var page paginated[UserGroup]
	if err := c.do(ctx, http.MethodGet, "/api/user-groups?pagination[limit]=100", nil, &page); err != nil {
		return nil, fmt.Errorf("listing user groups: %w", err)
	}
	return page.Data, nil
}

func (c *Client) CreateUserGroup(ctx context.Context, group UserGroup) (*UserGroup, error) {
	if c.apiKey == "" {
		return nil, ErrAPIKeyMissing{}
	}
	var out UserGroup
	if err := c.do(ctx, http.MethodPost, "/api/user-groups", group, &out); err != nil {
		return nil, fmt.Errorf("creating user group %q: %w", group.Name, err)
	}
	return &out, nil
}

func (c *Client) UpdateUserGroup(ctx context.Context, id string, group UserGroup) error {
	if c.apiKey == "" {
		return ErrAPIKeyMissing{}
	}
	if err := c.do(ctx, http.MethodPut, "/api/user-groups/"+url.PathEscape(id), group, nil); err != nil {
		return fmt.Errorf("updating user group %q: %w", id, err)
	}
	return nil
}

type paginated[T any] struct {
	Data []T `json:"data"`
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
