package reconciler

import (
	"context"
	"errors"
	"fmt"

	"github.com/iamkhattar/homelab/butler/internal/platform"
	"github.com/iamkhattar/homelab/butler/internal/pocketid"
	"github.com/iamkhattar/homelab/butler/internal/vault"
)

type PocketIDGroups struct {
	vault     *vault.Client
	resources platform.Resources
	url       string
}

func NewPocketIDGroups(vc *vault.Client, resources platform.Resources, url string) *PocketIDGroups {
	return &PocketIDGroups{vault: vc, resources: resources, url: url}
}

func (r *PocketIDGroups) Name() string { return "pocket-id-groups" }

func (r *PocketIDGroups) Reconcile(ctx context.Context) error {
	items, err := r.resources.ListPocketIDGroups(ctx)
	if err != nil {
		return fmt.Errorf("listing PocketIDGroups: %w", err)
	}
	if r.url == "" || len(items) == 0 {
		return nil
	}
	data, err := r.vault.ReadSecret(ctx, pocketid.ManagementCredentialVaultPath)
	apiKey, _ := data[pocketid.ManagementCredentialField].(string)
	if err != nil || apiKey == "" {
		waiting := errors.New("Pocket ID machine credential is unavailable")
		var failures []error
		for i := range items {
			if statusErr := convergeStatus(&items[i].Status, platform.Failed(items[i].Generation, "AwaitingAPIKey", waiting), func() error {
				return r.resources.UpdatePocketIDGroupStatus(ctx, &items[i])
			}); statusErr != nil {
				failures = append(failures, statusErr)
			}
		}
		return errors.Join(failures...)
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
	var failures []error
	for i := range items {
		providerID, err := r.reconcileOne(ctx, client, &items[i], byName)
		var desired platform.ResourceStatus
		if err != nil {
			desired = platform.Failed(items[i].Generation, "ReconcileFailed", err)
			failures = append(failures, err)
		} else {
			desired = platform.Ready(items[i].Generation, providerID)
		}
		if statusErr := convergeStatus(&items[i].Status, desired, func() error {
			return r.resources.UpdatePocketIDGroupStatus(ctx, &items[i])
		}); statusErr != nil {
			failures = append(failures, statusErr)
		}
	}
	return errors.Join(failures...)
}

func (r *PocketIDGroups) reconcileOne(ctx context.Context, client *pocketid.Client, item *platform.PocketIDGroup, existing map[string]pocketid.UserGroup) (string, error) {
	if item.Spec.FriendlyName == "" {
		return "", errors.New("friendlyName is required")
	}
	want := pocketid.UserGroup{Name: item.Name, FriendlyName: item.Spec.FriendlyName}
	have, ok := existing[item.Name]
	if !ok {
		created, err := client.CreateUserGroup(ctx, want)
		if err != nil {
			return "", err
		}
		return created.ID, nil
	}
	if have.FriendlyName != want.FriendlyName {
		if err := client.UpdateUserGroup(ctx, have.ID, want); err != nil {
			return "", err
		}
	}
	return have.ID, nil
}
