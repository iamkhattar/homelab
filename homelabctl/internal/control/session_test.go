package control

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSessionRoundTripAndExpiry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "session.json")
	now := time.Now().UTC()
	want := Session{Issuer: "https://auth.example", ClientID: "homelabctl", IDToken: "secret-token", ExpiresAt: now.Add(time.Hour)}
	if err := SaveSession(path, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	got, err := LoadSession(path, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.IDToken != want.IDToken || got.ClientID != want.ClientID {
		t.Fatalf("session = %#v", got)
	}
	if _, err := LoadSession(path, now.Add(2*time.Hour)); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("error = %v", err)
	}
	if err := RemoveSession(path); err != nil {
		t.Fatal(err)
	}
	if err := RemoveSession(path); err != nil {
		t.Fatal(err)
	}
}

func TestRandomURLToken(t *testing.T) {
	one, err := randomURLToken(32)
	if err != nil {
		t.Fatal(err)
	}
	two, err := randomURLToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if one == two || strings.ContainsAny(one, "+/=") {
		t.Fatalf("tokens are not independent URL-safe values")
	}
}
