package vault

import (
	"fmt"
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
