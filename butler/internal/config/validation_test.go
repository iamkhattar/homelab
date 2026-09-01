package config

import "testing"

func TestValidateControlPlaneEndpoints(t *testing.T) {
	t.Parallel()
	valid := Config{
		Certificates: CertificateConfig{ACMEDNSURL: "https://auth.acme-dns.io", Domain: "example.test", CredentialPath: "infrastructure/acme-dns", Namespace: "networking", CertificateName: "homelab-wildcard", TLSSecretName: "homelab-wildcard-tls"},
		OIDC:         OIDCConfig{Issuer: "https://auth.example.test", Audience: "butler", AdminURL: "http://pocket-id.security.svc.cluster.local"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"missing audience", func(c *Config) { c.OIDC.Audience = "" }},
		{"missing admin url", func(c *Config) { c.OIDC.AdminURL = "" }},
		{"insecure acme dns", func(c *Config) { c.Certificates.ACMEDNSURL = "http://auth.example.test" }},
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
