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
	groupIDs, err := resolveGroupIDs(ctx, pid, items)
	if err != nil {
		return err
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
		providerID, err := r.reconcileOne(ctx, pid, &items[i], byName, groupIDs)
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

func (r *PocketIDClients) reconcileOne(ctx context.Context, pid *pocketid.Client, item *platform.PocketIDClient, existing map[string]pocketid.OIDCClient, groupIDs map[string]string) (string, error) {
	if item.Spec.Type != "public" && item.Spec.Type != "confidential" {
		return "", errors.New("type must be public or confidential")
	}
	if len(item.Spec.RedirectURIs) == 0 || item.Spec.VaultPath == "" {
		return "", errors.New("redirectURIs and vaultPath are required")
	}
	have, present := existing[item.Name]
	currentGroups := have.AllowedUserGroups
	providerID, metadataChanged, err := r.reconcileClient(ctx, pid, item, have, present)
	if err != nil {
		return "", err
	}
	if err := reconcileAllowedGroups(ctx, pid, item, providerID, groupIDs, currentGroups, metadataChanged); err != nil {
		return "", err
	}
	return providerID, nil
}

// reconcileClient converges the OIDC client itself and its Vault material. The
// returned bool reports whether the client metadata was written, because the
// client update payload carries no allowed user groups and stale knowledge of
// the provider-side restriction must not be trusted afterwards.
func (r *PocketIDClients) reconcileClient(ctx context.Context, pid *pocketid.Client, item *platform.PocketIDClient, have pocketid.OIDCClient, present bool) (string, bool, error) {
	want := pocketid.OIDCClient{ID: item.Name, Name: item.Name, IsPublic: item.Spec.Type == "public", PKCEEnabled: item.Spec.Type == "public", CallbackURLs: item.Spec.RedirectURIs}
	if !present {
		created, err := pid.CreateClient(ctx, want)
		if err != nil {
			return "", false, fmt.Errorf("creating client: %w", err)
		}
		if err := r.persistCreds(ctx, item.Spec.VaultPath, created.ID, created.SecretID, created.Secret); err != nil {
			if created.SecretID != "" {
				cleanupErr := pid.DeleteClientSecret(ctx, created.ID, created.SecretID)
				return "", false, errors.Join(err, cleanupErr)
			}
			return "", false, err
		}
		return created.ID, false, nil
	}
	changed := false
	if !sameStringSet(have.CallbackURLs, want.CallbackURLs) || have.IsPublic != want.IsPublic || have.PKCEEnabled != want.PKCEEnabled {
		want.ID = have.ID
		if err := pid.UpdateClient(ctx, have.ID, want); err != nil {
			return "", false, fmt.Errorf("updating client: %w", err)
		}
		have = want
		changed = true
	}
	creds, err := r.vault.ReadSecretIfExists(ctx, item.Spec.VaultPath)
	if err != nil {
		return "", false, err
	}
	if have.IsPublic {
		secrets, err := pid.ListClientSecrets(ctx, have.ID)
		if err != nil {
			return "", false, err
		}
		for _, secret := range secrets {
			if err := pid.DeleteClientSecret(ctx, have.ID, secret.ID); err != nil {
				return "", false, err
			}
		}
		clientID, _ := creds["client_id"].(string)
		if len(creds) != 1 || clientID != have.ID {
			if err := r.persistCreds(ctx, item.Spec.VaultPath, have.ID, "", ""); err != nil {
				return "", false, err
			}
		}
		return have.ID, changed, nil
	}
	secretID, _ := creds["client_secret_id"].(string)
	secrets, err := pid.ListClientSecrets(ctx, have.ID)
	if err != nil {
		return "", false, err
	}
	if secretID == "" || !containsSecret(secrets, secretID) {
		created, createErr := pid.CreateSecret(ctx, have.ID)
		if createErr != nil {
			return "", false, fmt.Errorf("creating replacement client secret: %w", createErr)
		}
		if err := r.persistCreds(ctx, item.Spec.VaultPath, have.ID, created.ID, created.Secret); err != nil {
			cleanupErr := pid.DeleteClientSecret(ctx, have.ID, created.ID)
			return "", false, errors.Join(err, cleanupErr)
		}
		secretID = created.ID
	}
	for _, secret := range secrets {
		if secret.ID != secretID {
			if err := pid.DeleteClientSecret(ctx, have.ID, secret.ID); err != nil {
				return "", false, err
			}
		}
	}
	return have.ID, changed, nil
}

// resolveGroupIDs maps the Pocket ID group names declared by clients to their
// provider IDs. The names are resolved against the same provider group state
// that PocketIDGroups converges, which the scheduler runs before this
// reconciler, so a declared group exists by the time a client references it.
// Group names are not resolved from resource status: the provider list is the
// single source of truth for the IDs written back to Pocket ID.
func resolveGroupIDs(ctx context.Context, pid *pocketid.Client, items []platform.PocketIDClient) (map[string]string, error) {
	referenced := false
	for i := range items {
		if items[i].Spec.AllowedUserGroups != nil {
			referenced = true
			break
		}
	}
	if !referenced {
		return nil, nil
	}
	groups, err := pid.ListUserGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing pocket-id groups for client access restrictions: %w", err)
	}
	ids := make(map[string]string, len(groups))
	for _, group := range groups {
		ids[group.Name] = group.ID
	}
	return ids, nil
}

// reconcileAllowedGroups converges the client's Pocket-ID sign-in restriction.
//
// spec.allowedUserGroups is tri-state:
//   - absent/nil leaves the client unrestricted in the declarative sense: no
//     request is sent, so a restriction managed outside this repository is
//     preserved and Butler never opens a client by omission;
//   - an explicit empty list clears the restriction, which admits every user;
//   - a non-empty list admits only members of the named groups.
//
// The update fails closed. An unresolvable name aborts before any request,
// because sending the resolvable subset would silently grant or deny access to
// the wrong people, and sending an empty list would open the client to everyone.
func reconcileAllowedGroups(ctx context.Context, pid *pocketid.Client, item *platform.PocketIDClient, providerID string, groupIDs map[string]string, current []pocketid.UserGroup, clientMetadataChanged bool) error {
	names := item.Spec.AllowedUserGroups
	if names == nil {
		return nil
	}
	want := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		id, ok := groupIDs[name]
		if !ok || id == "" {
			return fmt.Errorf("allowedUserGroups references Pocket ID group %q, which does not exist", name)
		}
		want = append(want, id)
	}
	if !clientMetadataChanged {
		have := make([]string, 0, len(current))
		for _, group := range current {
			have = append(have, group.ID)
		}
		if sameStringSet(have, want) {
			return nil
		}
	}
	if err := pid.UpdateAllowedUserGroups(ctx, providerID, want); err != nil {
		return err
	}
	return nil
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
