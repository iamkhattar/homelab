// Package identity owns human and application identity management. Pocket ID
// is an adapter; API handlers depend on this domain service instead of provider
// routes directly.
package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/iamkhattar/homelab/butler/internal/platform"
	"github.com/iamkhattar/homelab/butler/internal/pocketid"
)

type SecretStore interface {
	ReadSecret(context.Context, string) (map[string]interface{}, error)
	WriteSecret(context.Context, string, map[string]interface{}) error
}

type Service struct {
	secrets SecretStore
	baseURL string
	clients ClientRegistry
}

type ClientRegistry interface {
	ListPocketIDClients(context.Context) ([]platform.PocketIDClient, error)
}

func NewService(secrets SecretStore, baseURL string, clients ClientRegistry) *Service {
	return &Service{secrets: secrets, baseURL: baseURL, clients: clients}
}

func (s *Service) client(ctx context.Context) (*pocketid.Client, error) {
	data, err := s.secrets.ReadSecret(ctx, pocketid.ManagementCredentialVaultPath)
	if err != nil {
		return nil, fmt.Errorf("loading Pocket ID management credential: %w", err)
	}
	key, _ := data[pocketid.ManagementCredentialField].(string)
	if strings.TrimSpace(key) == "" {
		return nil, pocketid.ErrAPIKeyMissing{}
	}
	return pocketid.NewClient(s.baseURL, key), nil
}

func (s *Service) ListUsers(ctx context.Context) ([]pocketid.User, error) {
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	return client.ListUsers(ctx)
}

func (s *Service) CreateUser(ctx context.Context, user pocketid.User) (*pocketid.User, error) {
	if err := validateUser(user); err != nil {
		return nil, err
	}
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	return client.CreateUser(ctx, user)
}

func (s *Service) UpdateUser(ctx context.Context, id string, user pocketid.User) (*pocketid.User, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("user id is required")
	}
	if err := validateUser(user); err != nil {
		return nil, err
	}
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	return client.UpdateUser(ctx, id, user)
}

func (s *Service) SetGroups(ctx context.Context, id string, groupIDs []string) (*pocketid.User, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("user id is required")
	}
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	return client.UpdateUserGroups(ctx, id, groupIDs)
}

func (s *Service) ListGroups(ctx context.Context) ([]pocketid.UserGroup, error) {
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	return client.ListUserGroups(ctx)
}

func (s *Service) ListClients(ctx context.Context) ([]pocketid.OIDCClient, error) {
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	return client.ListClients(ctx)
}

func (s *Service) RotateClientSecret(ctx context.Context, id string) error {
	client, err := s.client(ctx)
	if err != nil {
		return err
	}
	clients, err := client.ListClients(ctx)
	if err != nil {
		return err
	}
	var selected *pocketid.OIDCClient
	for i := range clients {
		if clients[i].ID == id {
			selected = &clients[i]
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("OIDC client %q was not found", id)
	}
	if selected.IsPublic {
		return fmt.Errorf("public OIDC clients do not have a client secret")
	}
	vaultPath, err := s.clientVaultPath(ctx, *selected)
	if err != nil {
		return err
	}
	existing, err := client.ListClientSecrets(ctx, selected.ID)
	if err != nil {
		return err
	}
	created, err := client.CreateSecret(ctx, selected.ID)
	if err != nil {
		return err
	}
	if err := s.secrets.WriteSecret(ctx, vaultPath, map[string]interface{}{
		"client_id": selected.ID, "client_secret": created.Secret, "client_secret_id": created.ID,
	}); err != nil {
		return errors.Join(err, client.DeleteClientSecret(ctx, selected.ID, created.ID))
	}
	var cleanup []error
	for _, secret := range existing {
		if err := client.DeleteClientSecret(ctx, selected.ID, secret.ID); err != nil {
			cleanup = append(cleanup, err)
		}
	}
	return errors.Join(cleanup...)
}

func (s *Service) clientVaultPath(ctx context.Context, selected pocketid.OIDCClient) (string, error) {
	if s.clients == nil {
		return "", fmt.Errorf("PocketIDClient registry is unavailable")
	}
	items, err := s.clients.ListPocketIDClients(ctx)
	if err != nil {
		return "", fmt.Errorf("listing PocketIDClient declarations: %w", err)
	}
	var matches []platform.PocketIDClient
	for i := range items {
		if items[i].Status.ProviderID == selected.ID || items[i].Name == selected.Name {
			matches = append(matches, items[i])
		}
	}
	if len(matches) != 1 || strings.TrimSpace(matches[0].Spec.VaultPath) == "" {
		return "", fmt.Errorf("OIDC client %q must map to exactly one PocketIDClient before rotation", selected.ID)
	}
	return matches[0].Spec.VaultPath, nil
}

func validateUser(user pocketid.User) error {
	if strings.TrimSpace(user.Username) == "" {
		return fmt.Errorf("username is required")
	}
	if user.IsAdmin {
		return fmt.Errorf("Butler does not create or promote Pocket ID administrators")
	}
	return nil
}
