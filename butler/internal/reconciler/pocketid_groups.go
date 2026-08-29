package reconciler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/iamkhattar/homelab/butler/internal/config"
	"github.com/iamkhattar/homelab/butler/internal/pocketid"
	"github.com/iamkhattar/homelab/butler/internal/vault"
)

// PocketIDGroups owns authorization-group metadata. It intentionally leaves
// user creation, passkey enrollment, and membership changes to Pocket ID.
type PocketIDGroups struct {
	vault  *vault.Client
	url    string
	groups []config.PocketIDGroupSpec
}

func NewPocketIDGroups(vc *vault.Client, url string, groups []config.PocketIDGroupSpec) *PocketIDGroups {
	return &PocketIDGroups{vault: vc, url: url, groups: groups}
}

func (r *PocketIDGroups) Name() string { return "pocket-id-groups" }

func (r *PocketIDGroups) Reconcile(ctx context.Context) error {
	if r.url == "" || len(r.groups) == 0 {
		return nil
	}
	data, err := r.vault.ReadSecret(ctx, adminAPIKeyVaultPath)
	if err != nil || data == nil {
		slog.Warn("pocket-id admin api key not configured; group provisioning disabled", "path", "secret/"+adminAPIKeyVaultPath)
		return nil
	}
	apiKey, _ := data[adminAPIKeyField].(string)
	if apiKey == "" {
		slog.Warn("pocket-id admin api key is empty; group provisioning disabled", "path", "secret/"+adminAPIKeyVaultPath)
		return nil
	}

	client := pocketid.NewClient(r.url, apiKey)
	existing, err := client.ListUserGroups(ctx)
	if err != nil {
		return err
	}
	byName := make(map[string]pocketid.UserGroup, len(existing))
	for _, group := range existing {
		byName[group.Name] = group
	}

	for _, spec := range r.groups {
		if spec.Name == "" || spec.FriendlyName == "" {
			return fmt.Errorf("pocket-id group name and friendlyName are required")
		}
		want := pocketid.UserGroup{Name: spec.Name, FriendlyName: spec.FriendlyName}
		have, ok := byName[spec.Name]
		if !ok {
			if _, err := client.CreateUserGroup(ctx, want); err != nil {
				return err
			}
			slog.Info("created pocket-id group", "name", spec.Name)
			continue
		}
		if have.FriendlyName != want.FriendlyName {
			if err := client.UpdateUserGroup(ctx, have.ID, want); err != nil {
				return err
			}
			slog.Info("updated pocket-id group", "name", spec.Name)
		}
	}
	return nil
}
