package control

import (
	"encoding/json"
	"os"
	"testing"

	"filippo.io/age"
)

func TestEncryptRecoveryBundleNeverWritesPlaintext(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/recovery.age"
	bundle := RecoveryBundle{Version: 1, Data: map[string]string{"root-token": "sensitive-value"}}
	if err := EncryptRecoveryBundle(path, identity.Recipient().String(), bundle); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == "" || json.Valid(raw) {
		t.Fatalf("bundle was empty or plaintext JSON")
	}
	if err := EncryptRecoveryBundle(path, identity.Recipient().String(), bundle); err == nil {
		t.Fatal("existing recovery bundle was overwritten")
	}
}
