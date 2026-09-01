package vault

import (
	"fmt"
	"strings"
	"testing"
)

func TestGenerateSecretData_Static(t *testing.T) {
	keys := map[string]KeySpec{
		"username": {Static: "admin"},
		"env":      {Static: "production"},
	}
	data, err := GenerateSecretData(keys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data["username"] != "admin" {
		t.Errorf("expected 'admin', got %v", data["username"])
	}
	if data["env"] != "production" {
		t.Errorf("expected 'production', got %v", data["env"])
	}
}

func TestGenerateSecretDataSupportsHexValues(t *testing.T) {
	got, err := GenerateSecretData(map[string]KeySpec{"rpc-secret": {HexLength: 32}})
	if err != nil {
		t.Fatal(err)
	}
	value, ok := got["rpc-secret"].(string)
	if !ok || len(value) != 64 {
		t.Fatalf("rpc-secret = %#v, want 64-character hex string", got["rpc-secret"])
	}
}

func TestGenerateSecretDataWithExistingPreservesAndBackfills(t *testing.T) {
	existing := map[string]interface{}{"password": "keep-me", "unmanaged": "also-keep"}
	keys := map[string]KeySpec{
		"password": {Length: 32},
		"username": {Static: "paperless"},
		"url":      {Template: "postgresql://{{username}}:{{password}}@postgres/paperless"},
	}

	got, err := GenerateSecretDataWithExisting(keys, existing)
	if err != nil {
		t.Fatal(err)
	}
	if got["password"] != "keep-me" {
		t.Fatalf("existing password was rotated: %v", got["password"])
	}
	if got["unmanaged"] != "also-keep" {
		t.Fatalf("unmanaged key was removed: %v", got["unmanaged"])
	}
	if got["username"] != "paperless" {
		t.Fatalf("missing static key was not added: %v", got["username"])
	}
	if got["url"] != "postgresql://paperless:keep-me@postgres/paperless" {
		t.Fatalf("template did not resolve against existing and new keys: %v", got["url"])
	}
}

func TestGenerateSecretData_Length(t *testing.T) {
	keys := map[string]KeySpec{
		"password": {Length: 32},
	}
	data, err := GenerateSecretData(keys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pw, ok := data["password"].(string)
	if !ok || pw == "" {
		t.Error("expected non-empty generated password")
	}
}

func TestGenerateSecretData_Template(t *testing.T) {
	keys := map[string]KeySpec{
		"user":     {Static: "root"},
		"password": {Static: "s3cret"},
		"dsn":      {Template: "postgres://{{user}}:{{password}}@localhost:5432/db"},
	}
	data, err := GenerateSecretData(keys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "postgres://root:s3cret@localhost:5432/db"
	if data["dsn"] != expected {
		t.Errorf("expected %q, got %v", expected, data["dsn"])
	}
}

func TestGenerateSecretDataResolvesTemplateDependencies(t *testing.T) {
	data, err := GenerateSecretData(map[string]KeySpec{
		"host":      {Static: "postgres.storage.svc"},
		"dsn":       {Template: "postgres://{{authority}}/app"},
		"authority": {Template: "user:pass@{{host}}"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if data["dsn"] != "postgres://user:pass@postgres.storage.svc/app" {
		t.Fatalf("unexpected dependent template: %v", data["dsn"])
	}
}

func TestGenerateSecretDataRejectsUnknownTemplateReference(t *testing.T) {
	_, err := GenerateSecretData(map[string]KeySpec{"dsn": {Template: "postgres://{{missing}}/app"}})
	if err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("error = %v", err)
	}
}

func TestGenerateSecretDataRejectsTemplateCycle(t *testing.T) {
	_, err := GenerateSecretData(map[string]KeySpec{
		"first":  {Template: "{{second}}"},
		"second": {Template: "{{first}}"},
	})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("error = %v", err)
	}
}

func TestGenerateSecretData_Mixed(t *testing.T) {
	keys := map[string]KeySpec{
		"username": {Static: "admin"},
		"password": {Length: 16},
		"url":      {Template: "https://{{username}}@example.com"},
	}
	data, err := GenerateSecretData(keys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 3 {
		t.Errorf("expected 3 keys, got %d", len(data))
	}
	if data["url"] != "https://admin@example.com" {
		t.Errorf("unexpected url: %v", data["url"])
	}
}

func TestGenerateSecretData_EmptySpec(t *testing.T) {
	keys := map[string]KeySpec{
		"bad": {},
	}
	_, err := GenerateSecretData(keys)
	if err == nil {
		t.Error("expected error for key with no length, static, or template")
	}
}

func TestGenerateSecretDataRejectsAmbiguousSpec(t *testing.T) {
	_, err := GenerateSecretData(map[string]KeySpec{
		"bad": {Length: 32, HexLength: 32},
	})
	if err == nil {
		t.Fatal("expected an error for multiple generation methods")
	}
}

func TestGenerateSecretData_Empty(t *testing.T) {
	data, err := GenerateSecretData(map[string]KeySpec{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty map, got %v", data)
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("some other error"), false},
		{fmt.Errorf("secret not found"), true},
		{fmt.Errorf("reading path: secret not found at data/foo"), true},
	}
	for _, tt := range tests {
		got := isNotFound(tt.err)
		if got != tt.want {
			t.Errorf("isNotFound(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}
