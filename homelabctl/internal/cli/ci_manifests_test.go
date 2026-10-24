package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAPIVersionGroup(t *testing.T) {
	cases := map[string]string{
		"v1":                            "",
		"apps/v1":                       "apps",
		"secrets.hashicorp.com/v1beta1": "secrets.hashicorp.com",
		"platform.6940469.xyz/v1alpha1": "platform.6940469.xyz",
		"":                              "",
	}
	for version, want := range cases {
		if got := apiVersionGroup(version); got != want {
			t.Errorf("apiVersionGroup(%q) = %q, want %q", version, got, want)
		}
	}
}

func TestAPIversionValueIgnoresTemplatesAndComments(t *testing.T) {
	cases := []struct {
		line   string
		want   string
		wantOK bool
	}{
		{"apiVersion: apps/v1", "apps/v1", true},
		{"  apiVersion: v1", "v1", true},
		{`apiVersion: "traefik.io/v1alpha1"`, "traefik.io/v1alpha1", true},
		{"apiVersion: {{ .Values.apiVersion }}", "", false},
		{"# apiVersion: apps/v1", "", false},
		{"kind: Deployment", "", false},
	}
	for _, c := range cases {
		got, ok := apiVersionValue(c.line)
		if got != c.want || ok != c.wantOK {
			t.Errorf("apiVersionValue(%q) = (%q, %v), want (%q, %v)", c.line, got, ok, c.want, c.wantOK)
		}
	}
}

func TestScanAPIGroupsFlagsUnknownGroup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifests.yaml")
	content := `apiVersion: apps/v1
kind: Deployment
---
apiVersion: secrets.hashicorp.com/v1beta1
kind: VaultStaticSecret
---
apiVersion: secrets.hashiicorp.com/v1beta1
kind: VaultStaticSecret
---
apiVersion: v1
kind: Secret
---
apiVersion: {{ .Values.group }}/v1
kind: Ignored
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	allowed := map[string]bool{"apps": true, "secrets.hashicorp.com": true}
	unknown := map[string][]string{}
	if err := scanAPIGroups(path, "cluster/example.yaml", allowed, unknown); err != nil {
		t.Fatal(err)
	}

	if len(unknown) != 1 {
		t.Fatalf("unknown = %v, want exactly the mistyped group", unknown)
	}
	locations, ok := unknown["secrets.hashiicorp.com"]
	if !ok {
		t.Fatalf("unknown = %v, want secrets.hashiicorp.com", unknown)
	}
	if len(locations) != 1 || locations[0] != "cluster/example.yaml:7" {
		t.Fatalf("locations = %v, want cluster/example.yaml:7", locations)
	}
}

func TestReadAPIGroupAllowlistSkipsComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "groups.txt")
	content := "# comment\n\napps\n  secrets.hashicorp.com  \n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	allowed, err := readAPIGroupAllowlist(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(allowed) != 2 || !allowed["apps"] || !allowed["secrets.hashicorp.com"] {
		t.Fatalf("allowed = %v", allowed)
	}
}
