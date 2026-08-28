package vault

import (
	"fmt"

	vaultapi "github.com/hashicorp/vault/api"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Client wraps the Vault API client.
type Client struct {
	raw *vaultapi.Client
}

// NewClient creates a Vault client pointing at the given address.
// No token is set initially — call SetToken after init/unseal.
func NewClient(addr string) (*Client, error) {
	cfg := vaultapi.DefaultConfig()
	cfg.Address = addr
	cfg.HttpClient.Transport = otelhttp.NewTransport(cfg.HttpClient.Transport)

	raw, err := vaultapi.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating vault client: %w", err)
	}

	return &Client{raw: raw}, nil
}

// SetToken sets the authentication token for subsequent requests.
func (c *Client) SetToken(token string) {
	c.raw.SetToken(token)
}

// Token returns the current token.
func (c *Client) Token() string {
	return c.raw.Token()
}

// Raw returns the underlying Vault API client for advanced operations.
func (c *Client) Raw() *vaultapi.Client {
	return c.raw
}
