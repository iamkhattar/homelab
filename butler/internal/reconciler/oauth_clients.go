package reconciler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/iamkhattar/homelab/butler/internal/platform"
	"github.com/iamkhattar/homelab/butler/internal/pocketid"
	"github.com/iamkhattar/homelab/butler/internal/vault"
)

// PocketIDClients reconciles namespaced PocketIDClient resources. Provider
// secrets go directly to Vault and are never written to resource status.
type PocketIDClients struct {
	vault     *vault.Client
	resources platform.Resources
	pidURL    string
}

func NewPocketIDClients(vc *vault.Client, resources platform.Resources, pidURL string) *PocketIDClients {
	return &PocketIDClients{vault: vc, resources: resources, pidURL: pidURL}
}

func (r *PocketIDClients) Name() string { return "pocket-id-clients" }

func (r *PocketIDClients) Reconcile(ctx context.Context) error {
	items, err := r.resources.ListPocketIDClients(ctx)
	if err != nil {
		return fmt.Errorf("listing PocketIDClients: %w", err)
	}
	if r.pidURL == "" || len(items) == 0 {
		return nil
	}

	apiKey, err := r.readAPIKey(ctx)
	if err != nil {
		slog.Warn("Pocket ID machine credential is not configured; client provisioning is waiting", "path", "secret/"+pocketid.ManagementCredentialVaultPath)
		var failures []error
		for i := range items {
			if statusErr := convergeStatus(&items[i].Status, platform.Failed(items[i].Generation, "AwaitingAPIKey", err), func() error {
				return r.resources.UpdatePocketIDClientStatus(ctx, &items[i])
			}); statusErr != nil {
				failures = append(failures, statusErr)
			}
		}
		return errors.Join(failures...)
	}

	pid := pocketid.NewClient(r.pidURL, apiKey)
	existing, err := pid.ListClients(ctx)
	if err != nil {
		return fmt.Errorf("listing pocket-id clients: %w", err)
	}
	byName := make(map[string]pocketid.OIDCClient, len(existing))
	for _, client := range existing {
		byName[client.Name] = client
	}
	nameCounts := make(map[string]int, len(items))
	for i := range items {
		nameCounts[items[i].Name]++
	}
	pathCounts, err := platform.VaultPathOwners(ctx, r.resources)
	if err != nil {
		return err
	}

	var failures []error
	for i := range items {
		if nameCounts[items[i].Name] > 1 {
			err := fmt.Errorf("Pocket ID client name %q must be unique across namespaces", items[i].Name)
			failures = append(failures, err)
			if statusErr := convergeStatus(&items[i].Status, platform.Failed(items[i].Generation, "DuplicateProviderName", err), func() error {
				return r.resources.UpdatePocketIDClientStatus(ctx, &items[i])
			}); statusErr != nil {
				failures = append(failures, statusErr)
			}
			continue
		}
		if pathCounts[items[i].Spec.VaultPath] > 1 {
			err := fmt.Errorf("Vault path %q must be owned by exactly one platform resource", items[i].Spec.VaultPath)
			failures = append(failures, err)
			if statusErr := convergeStatus(&items[i].Status, platform.Failed(items[i].Generation, "DuplicateVaultPath", err), func() error {
				return r.resources.UpdatePocketIDClientStatus(ctx, &items[i])
			}); statusErr != nil {
				failures = append(failures, statusErr)
			}
			continue
		}
		providerID, err := r.reconcileOne(ctx, pid, &items[i], byName)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s/%s: %w", items[i].Namespace, items[i].Name, err))
			if statusErr := convergeStatus(&items[i].Status, platform.Failed(items[i].Generation, "ReconcileFailed", err), func() error {
				return r.resources.UpdatePocketIDClientStatus(ctx, &items[i])
			}); statusErr != nil {
				failures = append(failures, statusErr)
			}
			continue
		}
		if err := convergeStatus(&items[i].Status, platform.Ready(items[i].Generation, providerID), func() error {
			return r.resources.UpdatePocketIDClientStatus(ctx, &items[i])
		}); err != nil {
			failures = append(failures, fmt.Errorf("updating %s/%s status: %w", items[i].Namespace, items[i].Name, err))
		}
	}
	return errors.Join(failures...)
}

func (r *PocketIDClients) reconcileOne(ctx context.Context, pid *pocketid.Client, item *platform.PocketIDClient, existing map[string]pocketid.OIDCClient) (string, error) {
	if item.Spec.Type != "public" && item.Spec.Type != "confidential" {
		return "", errors.New("type must be public or confidential")
	}
	if len(item.Spec.RedirectURIs) == 0 || item.Spec.VaultPath == "" {
		return "", errors.New("redirectURIs and vaultPath are required")
	}
	want := pocketid.OIDCClient{ID: item.Name, Name: item.Name, IsPublic: item.Spec.Type == "public", PKCEEnabled: item.Spec.Type == "public", CallbackURLs: item.Spec.RedirectURIs}
	have, present := existing[item.Name]
	if !present {
		created, err := pid.CreateClient(ctx, want)
		if err != nil {
			return "", fmt.Errorf("creating client: %w", err)
		}
		if err := r.persistCreds(ctx, item.Spec.VaultPath, created.ID, created.SecretID, created.Secret); err != nil {
			if created.SecretID != "" {
				cleanupErr := pid.DeleteClientSecret(ctx, created.ID, created.SecretID)
				return "", errors.Join(err, cleanupErr)
			}
			return "", err
		}
		return created.ID, nil
	}
	if !sameStringSet(have.CallbackURLs, want.CallbackURLs) || have.IsPublic != want.IsPublic || have.PKCEEnabled != want.PKCEEnabled {
		want.ID = have.ID
		if err := pid.UpdateClient(ctx, have.ID, want); err != nil {
			return "", fmt.Errorf("updating client: %w", err)
		}
		have = want
	}
	creds, err := r.vault.ReadSecretIfExists(ctx, item.Spec.VaultPath)
	if err != nil {
		return "", err
	}
	if have.IsPublic {
		secrets, err := pid.ListClientSecrets(ctx, have.ID)
		if err != nil {
			return "", err
		}
		for _, secret := range secrets {
			if err := pid.DeleteClientSecret(ctx, have.ID, secret.ID); err != nil {
				return "", err
			}
		}
		clientID, _ := creds["client_id"].(string)
		if len(creds) != 1 || clientID != have.ID {
			if err := r.persistCreds(ctx, item.Spec.VaultPath, have.ID, "", ""); err != nil {
				return "", err
			}
		}
		return have.ID, nil
	}
	secretID, _ := creds["client_secret_id"].(string)
	secrets, err := pid.ListClientSecrets(ctx, have.ID)
	if err != nil {
		return "", err
	}
	if secretID == "" || !containsSecret(secrets, secretID) {
		created, createErr := pid.CreateSecret(ctx, have.ID)
		if createErr != nil {
			return "", fmt.Errorf("creating replacement client secret: %w", createErr)
		}
		if err := r.persistCreds(ctx, item.Spec.VaultPath, have.ID, created.ID, created.Secret); err != nil {
			cleanupErr := pid.DeleteClientSecret(ctx, have.ID, created.ID)
			return "", errors.Join(err, cleanupErr)
		}
		secretID = created.ID
	}
	for _, secret := range secrets {
		if secret.ID != secretID {
			if err := pid.DeleteClientSecret(ctx, have.ID, secret.ID); err != nil {
				return "", err
			}
		}
	}
	return have.ID, nil
}

func (r *PocketIDClients) readAPIKey(ctx context.Context) (string, error) {
	data, err := r.vault.ReadSecret(ctx, pocketid.ManagementCredentialVaultPath)
	if err != nil {
		return "", err
	}
	key, _ := data[pocketid.ManagementCredentialField].(string)
	if key == "" {
		return "", errors.New("Pocket ID machine credential is unavailable")
	}
	return key, nil
}

func (r *PocketIDClients) persistCreds(ctx context.Context, path, clientID, clientSecretID, clientSecret string) error {
	data := map[string]interface{}{"client_id": clientID}
	if clientSecret != "" {
		data["client_secret"] = clientSecret
		data["client_secret_id"] = clientSecretID
	}
	return r.vault.WriteSecret(ctx, path, data)
}

func containsSecret(secrets []pocketid.OIDCClientSecret, id string) bool {
	for _, secret := range secrets {
		if secret.ID == id {
			return true
		}
	}
	return false
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, value := range a {
		set[value] = struct{}{}
	}
	for _, value := range b {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}
