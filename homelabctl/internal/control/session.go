package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const sessionFileName = "session.json"

type Session struct {
	Issuer    string    `json:"issuer"`
	ClientID  string    `json:"clientId"`
	IDToken   string    `json:"idToken"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func SessionPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolving user configuration directory: %w", err)
	}
	return filepath.Join(base, "homelabctl", sessionFileName), nil
}

func SaveSession(path string, session Session) error {
	if session.IDToken == "" || session.ExpiresAt.IsZero() {
		return fmt.Errorf("OIDC session is incomplete")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating session directory: %w", err)
	}
	raw, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("encoding OIDC session: %w", err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, raw, 0o600); err != nil {
		return fmt.Errorf("writing OIDC session: %w", err)
	}
	if err := os.Chmod(temporary, 0o600); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("securing OIDC session: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("installing OIDC session: %w", err)
	}
	return nil
}

func LoadSession(path string, now time.Time) (Session, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- path is the fixed user configuration session path.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Session{}, fmt.Errorf("no Pocket ID session; run homelabctl control login")
		}
		return Session{}, fmt.Errorf("reading OIDC session: %w", err)
	}
	var session Session
	if err := json.Unmarshal(raw, &session); err != nil {
		return Session{}, fmt.Errorf("decoding OIDC session: %w", err)
	}
	if session.IDToken == "" || !session.ExpiresAt.After(now.Add(time.Minute)) {
		return Session{}, fmt.Errorf("Pocket ID session expired; run homelabctl control login")
	}
	return session, nil
}

func RemoveSession(path string) error {
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing OIDC session: %w", err)
	}
	return nil
}
