package certificates

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const accountJSONKey = "acmedns.json"

// SecretStore is the narrow Vault contract required by certificate bootstrap.
type SecretStore interface {
	ReadSecretIfExists(context.Context, string) (map[string]interface{}, error)
	WriteSecret(context.Context, string, map[string]interface{}) error
}

// Config describes the single public certificate authority integration Butler owns.
type Config struct {
	APIURL          string
	Domain          string
	CredentialPath  string
	CertificateNS   string
	CertificateName string
	TLSSecretName   string
}

// Registration is returned by acme-dns. Password is deliberately omitted from Status.
type Registration struct {
	Username   string   `json:"username"`
	Password   string   `json:"password"` // #nosec G117 -- acme-dns requires this field in cert-manager's Vault-backed account document.
	FullDomain string   `json:"fulldomain"`
	Subdomain  string   `json:"subdomain"`
	AllowFrom  []string `json:"allowfrom"`
}

type Status struct {
	ProviderURL      string `json:"providerUrl"`
	Domain           string `json:"domain"`
	CNAMEHost        string `json:"cnameHost"`
	CNAMETarget      string `json:"cnameTarget,omitempty"`
	Registered       bool   `json:"registered"`
	DelegationValid  bool   `json:"delegationValid"`
	CertificateReady bool   `json:"certificateReady"`
}

type Registrar interface {
	Register(context.Context) (Registration, error)
}

type Resolver interface {
	LookupCNAME(context.Context, string) (string, error)
}

type Client struct {
	baseURL *url.URL
	http    *http.Client
}

func NewClient(rawURL string) (*Client, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return nil, fmt.Errorf("acme-dns API URL must be an absolute HTTPS URL")
	}
	return &Client{baseURL: parsed, http: &http.Client{Timeout: 15 * time.Second}}, nil
}

func (c *Client) Register(ctx context.Context) (Registration, error) {
	endpoint := c.baseURL.ResolveReference(&url.URL{Path: strings.TrimRight(c.baseURL.Path, "/") + "/register"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader([]byte(`{}`)))
	if err != nil {
		return Registration{}, fmt.Errorf("creating acme-dns registration request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return Registration{}, fmt.Errorf("registering acme-dns account: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return Registration{}, fmt.Errorf("registering acme-dns account: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var registration Registration
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<10)).Decode(&registration); err != nil {
		return Registration{}, fmt.Errorf("decoding acme-dns registration: %w", err)
	}
	if err := validateRegistration(registration); err != nil {
		return Registration{}, err
	}
	return registration, nil
}

type Manager struct {
	config    Config
	store     SecretStore
	registrar Registrar
	resolver  Resolver
}

func NewManager(config Config, store SecretStore, registrar Registrar, resolver Resolver) (*Manager, error) {
	if strings.TrimSpace(config.Domain) == "" || strings.TrimSpace(config.CredentialPath) == "" {
		return nil, fmt.Errorf("certificate domain and Vault credential path are required")
	}
	if registrar == nil || store == nil {
		return nil, fmt.Errorf("certificate registrar and secret store are required")
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return &Manager{config: config, store: store, registrar: registrar, resolver: resolver}, nil
}

func (m *Manager) EnsureRegistration(ctx context.Context) (Status, error) {
	existing, err := m.store.ReadSecretIfExists(ctx, m.config.CredentialPath)
	if err != nil {
		return Status{}, fmt.Errorf("reading acme-dns credential: %w", err)
	}
	if len(existing) > 0 {
		registration, err := registrationFromSecret(existing, m.config.Domain)
		if err != nil {
			return Status{}, err
		}
		return m.status(registration), nil
	}
	registration, err := m.registrar.Register(ctx)
	if err != nil {
		return Status{}, err
	}
	account := map[string]Registration{m.config.Domain: registration}
	// #nosec G117 -- this is the exact Vault-backed account document consumed
	// by cert-manager; it is never logged or returned by Butler's API.
	raw, err := json.Marshal(account)
	if err != nil {
		return Status{}, fmt.Errorf("encoding acme-dns credential: %w", err)
	}
	if err := m.store.WriteSecret(ctx, m.config.CredentialPath, map[string]interface{}{
		accountJSONKey: string(raw),
		"fulldomain":   registration.FullDomain,
		"provider-url": m.config.APIURL,
	}); err != nil {
		return Status{}, fmt.Errorf("storing acme-dns credential: %w", err)
	}
	return m.status(registration), nil
}

func (m *Manager) VerifyDelegation(ctx context.Context) (Status, error) {
	status, err := m.EnsureRegistration(ctx)
	if err != nil {
		return Status{}, err
	}
	resolved, err := m.resolver.LookupCNAME(ctx, status.CNAMEHost)
	if err != nil {
		return status, fmt.Errorf("resolving %s CNAME: %w", status.CNAMEHost, err)
	}
	if normalizeDNSName(resolved) != normalizeDNSName(status.CNAMETarget) {
		return status, fmt.Errorf("%s resolves to %s, expected %s", status.CNAMEHost, resolved, status.CNAMETarget)
	}
	status.DelegationValid = true
	return status, nil
}

func (m *Manager) Status(ctx context.Context) (Status, error) {
	existing, err := m.store.ReadSecretIfExists(ctx, m.config.CredentialPath)
	if err != nil {
		return Status{}, err
	}
	if len(existing) == 0 {
		return Status{ProviderURL: m.config.APIURL, Domain: m.config.Domain, CNAMEHost: "_acme-challenge." + m.config.Domain}, nil
	}
	registration, err := registrationFromSecret(existing, m.config.Domain)
	if err != nil {
		return Status{}, err
	}
	return m.status(registration), nil
}

func (m *Manager) status(registration Registration) Status {
	return Status{
		ProviderURL: m.config.APIURL,
		Domain:      m.config.Domain,
		CNAMEHost:   "_acme-challenge." + m.config.Domain,
		CNAMETarget: registration.FullDomain,
		Registered:  true,
	}
}

func (m *Manager) TLSSecretRef() (string, string) {
	return m.config.CertificateNS, m.config.TLSSecretName
}

func registrationFromSecret(secret map[string]interface{}, domain string) (Registration, error) {
	raw, ok := secret[accountJSONKey].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return Registration{}, fmt.Errorf("stored acme-dns credential is missing %s", accountJSONKey)
	}
	var accounts map[string]Registration
	if err := json.Unmarshal([]byte(raw), &accounts); err != nil {
		return Registration{}, fmt.Errorf("decoding stored acme-dns credential: %w", err)
	}
	registration, ok := accounts[domain]
	if !ok {
		return Registration{}, fmt.Errorf("stored acme-dns credential has no account for %s", domain)
	}
	if err := validateRegistration(registration); err != nil {
		return Registration{}, err
	}
	return registration, nil
}

func validateRegistration(registration Registration) error {
	if strings.TrimSpace(registration.Username) == "" || strings.TrimSpace(registration.Password) == "" ||
		strings.TrimSpace(registration.FullDomain) == "" || strings.TrimSpace(registration.Subdomain) == "" {
		return fmt.Errorf("acme-dns registration returned incomplete credentials")
	}
	return nil
}

func normalizeDNSName(value string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
}
