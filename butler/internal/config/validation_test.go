package config

import "testing"

func TestValidateControlPlaneEndpoints(t *testing.T) {
	t.Parallel()
	valid := Config{
		Certificates: CertificateConfig{ACMEDNSURL: "https://auth.acme-dns.io", Domain: "example.test", CredentialPath: "infrastructure/acme-dns", Namespace: "networking", CertificateName: "homelab-wildcard", TLSSecretName: "homelab-wildcard-tls"},
		OIDC:         OIDCConfig{Issuer: "https://auth.example.test", AllowedClients: []string{"homelabctl"}, Groups: OIDCGroupsConfig{Admin: "butleradmin", Viewer: "butlerviewer"}, AdminURL: "http://pocket-id.security.svc.cluster.local"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"missing allowed clients", func(c *Config) { c.OIDC.AllowedClients = nil }},
		{"missing admin url", func(c *Config) { c.OIDC.AdminURL = "" }},
		{"insecure acme dns", func(c *Config) { c.Certificates.ACMEDNSURL = "http://auth.example.test" }},
		{"missing admin group", func(c *Config) { c.OIDC.Groups.Admin = "" }},
		{"duplicate viewer group", func(c *Config) { c.OIDC.Groups.Viewer = c.OIDC.Groups.Admin }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := valid
			tt.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateOIDCGroupsAllowsADisabledTier(t *testing.T) {
	t.Parallel()
	// The operator tier is disabled in production by naming no group. An empty
	// name must be accepted while admin and viewer still resolve.
	cfg := Config{
		Certificates: CertificateConfig{ACMEDNSURL: "https://auth.acme-dns.io", Domain: "example.test", CredentialPath: "infrastructure/acme-dns", Namespace: "networking", CertificateName: "homelab-wildcard", TLSSecretName: "homelab-wildcard-tls"},
		OIDC:         OIDCConfig{Issuer: "https://auth.example.test", AllowedClients: []string{"homelabctl"}, Groups: OIDCGroupsConfig{Admin: "butleradmin", Operator: "", Viewer: "butlerviewer"}, AdminURL: "http://pocket-id.security.svc.cluster.local"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("empty operator group must be valid: %v", err)
	}
}

func TestValidateOIDCGroupsRejectsDuplicateNames(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Certificates: CertificateConfig{ACMEDNSURL: "https://auth.acme-dns.io", Domain: "example.test", CredentialPath: "infrastructure/acme-dns", Namespace: "networking", CertificateName: "homelab-wildcard", TLSSecretName: "homelab-wildcard-tls"},
		OIDC:         OIDCConfig{Issuer: "https://auth.example.test", AllowedClients: []string{"homelabctl"}, Groups: OIDCGroupsConfig{Admin: "butleradmin", Operator: "butleradmin", Viewer: "butlerviewer"}, AdminURL: "http://pocket-id.security.svc.cluster.local"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("two tiers naming the same group must be rejected")
	}
}
