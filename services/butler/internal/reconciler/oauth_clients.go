package reconciler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/iamkhattar/homelab/services/butler/internal/config"
	"github.com/iamkhattar/homelab/services/butler/internal/pocketid"
	"github.com/iamkhattar/homelab/services/butler/internal/vault"
)

// adminAPIKeyVaultPath is where the Pocket-ID admin API key lives. An
// operator generates the key in the Pocket-ID UI once on first install and
// stores it here; butler reads it for every reconcile.
const (
	adminAPIKeyVaultPath = "pocket-id/admin"
	adminAPIKeyField     = "api-key"
	oauthClientPathFmt   = "oauth/%s"
)

// OAuthClients ensures every OIDC client declared in butler's config exists
// in Pocket-ID and that the {client_id, client_secret} pair is persisted at
// secret/oauth/<name>. Other charts (vault, butler itself, applications)
// consume those creds via VaultStaticSecret.
type OAuthClients struct {
	vault  *vault.Client
	specs  []config.OAuthClientSpec
	pidURL string // base URL of Pocket-ID, e.g. https://auth.shivlab.com
}

// NewOAuthClients builds an OAuthClients reconciler. If pidURL is empty,
// reconciliation no-ops (used during bootstrap before Pocket-ID is up).
func NewOAuthClients(vc *vault.Client, pidURL string, specs []config.OAuthClientSpec) *OAuthClients {
	return &OAuthClients{vault: vc, specs: specs, pidURL: pidURL}
}

// Name implements Reconciler.
func (r *OAuthClients) Name() string { return "oauth-clients" }

// Reconcile ensures each declared client exists in Pocket-ID with the
// right callback URLs, and that its credentials are stored in Vault.
//
// Three failure modes are handled gracefully (logged + reconciliation
// continues to the next pass):
//   - No Pocket-ID URL configured yet (Phase 1A bootstrap): no-op.
//   - No admin API key in Vault yet: log a one-time warning. Operator must
//     create one in Pocket-ID UI and store at secret/pocket-id/admin.
//   - Pocket-ID is unreachable: surface as error so the scheduler retries.
func (r *OAuthClients) Reconcile(ctx context.Context) error {
	if r.pidURL == "" || len(r.specs) == 0 {
		return nil
	}

	apiKey, err := r.readAPIKey(ctx)
	if err != nil {
		// Soft-fail: operator hasn't bootstrapped the admin key yet.
		slog.Warn("pocket-id admin api key not configured; oauth client provisioning disabled",
			"path", "secret/"+adminAPIKeyVaultPath, "err", err)
		return nil
	}

	pid := pocketid.NewClient(r.pidURL, apiKey)

	existing, err := pid.ListClients(ctx)
	if err != nil {
		return fmt.Errorf("listing pocket-id clients: %w", err)
	}
	byName := make(map[string]pocketid.OIDCClient, len(existing))
	for _, c := range existing {
		byName[c.Name] = c
	}

	for _, spec := range r.specs {
		if err := r.reconcileOne(ctx, pid, spec, byName); err != nil {
			// Continue with the rest of the specs even if one fails — we
			// don't want a single broken client to block the others.
			slog.Error("reconciling oauth client failed", "name", spec.Name, "err", err)
		}
	}
	return nil
}

func (r *OAuthClients) reconcileOne(
	ctx context.Context,
	pid *pocketid.Client,
	spec config.OAuthClientSpec,
	existing map[string]pocketid.OIDCClient,
) error {
	want := pocketid.OIDCClient{
		Name:         spec.Name,
		IsPublic:     spec.Kind == "public",
		CallbackURLs: spec.RedirectURIs,
	}

	have, present := existing[spec.Name]
	if !present {
		created, err := pid.CreateClient(ctx, want)
		if err != nil {
			return fmt.Errorf("creating client: %w", err)
		}
		slog.Info("created pocket-id oauth client", "name", spec.Name, "id", created.ID)
		return r.persistCreds(ctx, spec.Name, created.ID, created.Secret)
	}

	// Detect drift in callback URLs / name shape. Pocket-ID doesn't return
	// the secret on read so we only update the metadata; the existing
	// secret stays in Vault from the previous run.
	if !sameStringSet(have.CallbackURLs, want.CallbackURLs) || have.IsPublic != want.IsPublic {
		want.ID = have.ID
		if err := pid.UpdateClient(ctx, have.ID, want); err != nil {
			return fmt.Errorf("updating client: %w", err)
		}
		slog.Info("updated pocket-id oauth client", "name", spec.Name, "id", have.ID)
	}

	// If we have a client_id in Pocket-ID but no creds in Vault (e.g.
	// because someone manually deleted the Vault path), rotate the
	// secret and persist.
	hasCreds, err := r.vault.SecretExists(ctx, fmt.Sprintf(oauthClientPathFmt, spec.Name))
	if err != nil {
		return fmt.Errorf("checking vault creds: %w", err)
	}
	if !hasCreds {
		newSecret, err := pid.RotateSecret(ctx, have.ID)
		if err != nil {
			return fmt.Errorf("rotating secret: %w", err)
		}
		slog.Info("rotated pocket-id oauth client secret to repopulate vault", "name", spec.Name)
		return r.persistCreds(ctx, spec.Name, have.ID, newSecret)
	}
	return nil
}

func (r *OAuthClients) readAPIKey(ctx context.Context) (string, error) {
	data, err := r.vault.ReadSecret(ctx, adminAPIKeyVaultPath)
	if err != nil {
		return "", err
	}
	if data == nil {
		return "", errors.New("no data at secret/" + adminAPIKeyVaultPath)
	}
	key, ok := data[adminAPIKeyField].(string)
	if !ok || key == "" {
		return "", fmt.Errorf("missing field %q at secret/%s", adminAPIKeyField, adminAPIKeyVaultPath)
	}
	return key, nil
}

func (r *OAuthClients) persistCreds(ctx context.Context, name, clientID, clientSecret string) error {
	return r.vault.WriteSecret(ctx, fmt.Sprintf(oauthClientPathFmt, name), map[string]interface{}{
		"client_id":     clientID,
		"client_secret": clientSecret,
	})
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, s := range a {
		set[s] = struct{}{}
	}
	for _, s := range b {
		if _, ok := set[s]; !ok {
			return false
		}
	}
	return true
}
