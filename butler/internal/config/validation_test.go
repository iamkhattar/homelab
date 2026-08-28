package config

import "testing"

func TestValidatePocketIDControlPlane(t *testing.T) {
	t.Parallel()
	valid := Config{
		OIDC:           OIDCConfig{Issuer: "https://auth.example.test", Audience: "butler", AdminURL: "http://pocket-id.security.svc.cluster.local"},
		PocketIDGroups: []PocketIDGroupSpec{{Name: "homelab-admin", FriendlyName: "Administrators"}},
		OAuthClients:   []OAuthClientSpec{{Name: "butler", Kind: "public", RedirectURIs: []string{"https://butler.example.test/auth/callback"}}},
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
		{"bad group", func(c *Config) { c.PocketIDGroups[0].Name = "Admin Group" }},
		{"bad client kind", func(c *Config) { c.OAuthClients[0].Kind = "native" }},
		{"relative redirect", func(c *Config) { c.OAuthClients[0].RedirectURIs[0] = "/callback" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := valid
			candidate.PocketIDGroups = append([]PocketIDGroupSpec(nil), valid.PocketIDGroups...)
			candidate.OAuthClients = append([]OAuthClientSpec(nil), valid.OAuthClients...)
			candidate.OAuthClients[0].RedirectURIs = append([]string(nil), valid.OAuthClients[0].RedirectURIs...)
			tt.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
