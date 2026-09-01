package reconciler

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/iamkhattar/homelab/butler/internal/platform"
	"github.com/iamkhattar/homelab/butler/internal/vault"
)

// ManagedCredentials reconciles application-owned generation and projection
// policies into Vault. Kubernetes resources contain policy only, never values.
type ManagedCredentials struct {
	vault     *vault.Client
	resources platform.Resources
}

func NewManagedCredentials(vc *vault.Client, resources platform.Resources) *ManagedCredentials {
	return &ManagedCredentials{vault: vc, resources: resources}
}

func (r *ManagedCredentials) Name() string { return "managed-credentials" }

func (r *ManagedCredentials) Reconcile(ctx context.Context) error {
	items, err := r.resources.ListManagedCredentials(ctx)
	if err != nil {
		return fmt.Errorf("listing ManagedCredentials: %w", err)
	}
	var failures []error
	pathCounts, err := platform.VaultPathOwners(ctx, r.resources)
	if err != nil {
		return err
	}
	for i := range items {
		if pathCounts[items[i].Spec.VaultPath] > 1 {
			err := fmt.Errorf("Vault path %q must be owned by exactly one platform resource", items[i].Spec.VaultPath)
			failures = append(failures, err)
			if statusErr := convergeStatus(&items[i].Status, platform.Failed(items[i].Generation, "DuplicateVaultPath", err), func() error {
				return r.resources.UpdateManagedCredentialStatus(ctx, &items[i])
			}); statusErr != nil {
				failures = append(failures, statusErr)
			}
			continue
		}
		if err := r.reconcileOne(ctx, &items[i]); err != nil {
			failures = append(failures, fmt.Errorf("%s/%s: %w", items[i].Namespace, items[i].Name, err))
			if statusErr := convergeStatus(&items[i].Status, platform.Failed(items[i].Generation, "ReconcileFailed", err), func() error {
				return r.resources.UpdateManagedCredentialStatus(ctx, &items[i])
			}); statusErr != nil {
				failures = append(failures, statusErr)
			}
			continue
		}
		if err := convergeStatus(&items[i].Status, platform.Ready(items[i].Generation, ""), func() error {
			return r.resources.UpdateManagedCredentialStatus(ctx, &items[i])
		}); err != nil {
			failures = append(failures, fmt.Errorf("updating %s/%s status: %w", items[i].Namespace, items[i].Name, err))
		}
	}
	return errors.Join(failures...)
}

func (r *ManagedCredentials) reconcileOne(ctx context.Context, item *platform.ManagedCredential) error {
	if item.Spec.VaultPath == "" || len(item.Spec.Fields) == 0 {
		return errors.New("vaultPath and at least one field are required")
	}
	existing, err := r.vault.ReadSecretIfExists(ctx, item.Spec.VaultPath)
	if err != nil {
		return err
	}
	seed := make(map[string]interface{}, len(existing)+len(item.Spec.Fields))
	for key, value := range existing {
		seed[key] = value
	}
	generated := make(map[string]vault.KeySpec)

	for name, field := range item.Spec.Fields {
		methods := 0
		for _, set := range []bool{field.Generate != nil, field.Value != nil, field.Template != "", field.SourceRef != nil} {
			if set {
				methods++
			}
		}
		if methods != 1 {
			return fmt.Errorf("field %s must configure exactly one value source", name)
		}
		switch {
		case field.Value != nil:
			seed[name] = *field.Value
		case field.SourceRef != nil:
			if field.SourceRef.Path == "" || field.SourceRef.Key == "" {
				return fmt.Errorf("field %s sourceRef requires path and key", name)
			}
			source, err := r.vault.ReadSecret(ctx, field.SourceRef.Path)
			if err != nil {
				return err
			}
			value, ok := source[field.SourceRef.Key]
			if !ok {
				return fmt.Errorf("source %s is missing key %s", field.SourceRef.Path, field.SourceRef.Key)
			}
			seed[name] = value
		case field.Template != "":
			generated[name] = vault.KeySpec{Template: field.Template}
		case field.Generate != nil:
			if field.Generate.Length < 1 {
				return fmt.Errorf("field %s generator length must be positive", name)
			}
			switch field.Generate.Type {
			case "password":
				generated[name] = vault.KeySpec{Length: field.Generate.Length}
			case "hex":
				generated[name] = vault.KeySpec{HexLength: field.Generate.Length}
			default:
				return fmt.Errorf("field %s generator type must be password or hex", name)
			}
		}
	}

	want, err := vault.GenerateSecretDataWithExisting(generated, seed)
	if err != nil {
		return err
	}
	if reflect.DeepEqual(existing, want) {
		return nil
	}
	return r.vault.WriteSecret(ctx, item.Spec.VaultPath, want)
}
