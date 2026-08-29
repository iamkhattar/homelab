package control

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"filippo.io/age"
)

type RecoveryBundle struct {
	Version    int               `json:"version"`
	Context    string            `json:"context"`
	Namespace  string            `json:"namespace"`
	SecretName string            `json:"secretName"`
	ExportedAt time.Time         `json:"exportedAt"`
	Data       map[string]string `json:"data"`
}

// EncryptRecoveryBundle writes an age-encrypted recovery bundle without ever
// materializing plaintext recovery credentials on disk.
func EncryptRecoveryBundle(path, recipient string, bundle RecoveryBundle) error {
	parsed, err := age.ParseX25519Recipient(recipient)
	if err != nil {
		return fmt.Errorf("parsing age recipient: %w", err)
	}
	// #nosec G304 -- the CLI resolves and validates this operator-selected path
	// outside the repository; O_EXCL also refuses existing files and symlinks.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("creating recovery bundle: %w", err)
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	writer, err := age.Encrypt(file, parsed)
	if err != nil {
		return fmt.Errorf("starting recovery encryption: %w", err)
	}
	if err := json.NewEncoder(writer).Encode(bundle); err != nil {
		return fmt.Errorf("encrypting recovery bundle: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finalizing recovery encryption: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("closing recovery bundle: %w", err)
	}
	remove = false
	return nil
}
