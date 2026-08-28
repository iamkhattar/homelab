package reconciler

import (
	"context"
	"fmt"
	"reflect"

	"github.com/iamkhattar/homelab/butler/internal/config"
	"github.com/iamkhattar/homelab/butler/internal/vault"
)

// ManagedCredentials publishes consumer-specific projections of shared
// credentials. The destination is replaced only when its managed view drifts.
type ManagedCredentials struct {
	vault *vault.Client
	specs []config.ManagedCredentialSpec
}

func NewManagedCredentials(vc *vault.Client, specs []config.ManagedCredentialSpec) *ManagedCredentials {
	return &ManagedCredentials{vault: vc, specs: specs}
}

func (r *ManagedCredentials) Name() string { return "managed-credentials" }

func (r *ManagedCredentials) Reconcile(ctx context.Context) error {
	for _, spec := range r.specs {
		if spec.SourcePath == "" || spec.DestinationPath == "" {
			return fmt.Errorf("managed credential requires sourcePath and destinationPath")
		}
		source, err := r.vault.ReadSecret(ctx, spec.SourcePath)
		if err != nil {
			return fmt.Errorf("reading managed credential source %s: %w", spec.SourcePath, err)
		}
		want := make(map[string]interface{}, len(spec.Keys)+len(spec.Static))
		for destinationKey, sourceKey := range spec.Keys {
			value, ok := source[sourceKey]
			if !ok {
				return fmt.Errorf("managed credential source %s is missing key %s", spec.SourcePath, sourceKey)
			}
			want[destinationKey] = value
		}
		for key, value := range spec.Static {
			want[key] = value
		}

		have, err := r.vault.ReadSecretIfExists(ctx, spec.DestinationPath)
		if err != nil {
			return err
		}
		if reflect.DeepEqual(have, want) {
			continue
		}
		if err := r.vault.WriteSecret(ctx, spec.DestinationPath, want); err != nil {
			return err
		}
	}
	return nil
}
