package reconciler

import (
	"context"
	"log/slog"

	"github.com/iamkhattar/homelab/services/butler/internal/config"
	"github.com/iamkhattar/homelab/services/butler/internal/vault"
)

// Secrets ensures all declared secrets exist in Vault.
type Secrets struct {
	vault   *vault.Client
	secrets []config.SecretSpec
}

// NewSecrets creates a new Secrets reconciler.
func NewSecrets(vc *vault.Client, secrets []config.SecretSpec) *Secrets {
	return &Secrets{vault: vc, secrets: secrets}
}

func (r *Secrets) Name() string { return "secrets" }

func (r *Secrets) Reconcile(ctx context.Context) error {
	for _, spec := range r.secrets {
		exists, err := r.vault.SecretExists(ctx, spec.Path)
		if err != nil {
			return err
		}
		if exists {
			slog.Debug("secret already exists, skipping", "path", spec.Path)
			continue
		}

		keys := make(map[string]vault.KeySpec, len(spec.Keys))
		for name, kc := range spec.Keys {
			keys[name] = vault.KeySpec{
				Length:   kc.Length,
				Static:   kc.Static,
				Template: kc.Template,
			}
		}

		data, err := vault.GenerateSecretData(keys)
		if err != nil {
			return err
		}

		if err := r.vault.WriteSecret(ctx, spec.Path, data); err != nil {
			return err
		}
	}
	return nil
}
