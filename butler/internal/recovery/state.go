package recovery

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/iamkhattar/homelab/butler/internal/vault"
)

const initSecretName = "butler-vault-init"
const stateConfigMapName = "butler-bootstrap-state"

type Bootstrapper interface {
	Reconcile(context.Context) error
}

func (s *Service) ImportPocketIDAPIKey(ctx context.Context, apiKey string) error {
	if strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("Pocket ID API key is required")
	}
	if len(apiKey) > 4096 {
		return fmt.Errorf("Pocket ID API key is too large")
	}
	if err := s.vault.WriteSecret(ctx, "pocket-id/admin", map[string]interface{}{"api-key": apiKey}); err != nil {
		return fmt.Errorf("storing Pocket ID API key: %w", err)
	}
	return nil
}

type Status struct {
	Mode                string    `json:"mode"`
	Phase               string    `json:"phase"`
	VaultReachable      bool      `json:"vaultReachable"`
	VaultInitialized    bool      `json:"vaultInitialized"`
	VaultSealed         bool      `json:"vaultSealed"`
	RecoverySecretFound bool      `json:"recoverySecretFound"`
	ButlerLoginVerified bool      `json:"butlerLoginVerified"`
	VaultLoginVerified  bool      `json:"vaultLoginVerified"`
	CheckedAt           time.Time `json:"checkedAt"`
	Error               string    `json:"error,omitempty"`
}

type Service struct {
	vault                *vault.Client
	k8s                  kubernetes.Interface
	namespace            string
	bootstrapper         Bootstrapper
	identityBootstrapper Bootstrapper
}

func NewService(vc *vault.Client, k8s kubernetes.Interface, namespace string, bootstrapper Bootstrapper, identity ...Bootstrapper) *Service {
	service := &Service{vault: vc, k8s: k8s, namespace: namespace, bootstrapper: bootstrapper}
	if len(identity) > 0 {
		service.identityBootstrapper = identity[0]
	}
	return service
}

func (s *Service) Status(ctx context.Context) Status {
	status := Status{Mode: "recovery", Phase: "unavailable", CheckedAt: time.Now().UTC()}
	vaultStatus, err := s.vault.Status(ctx)
	if err != nil {
		status.Error = err.Error()
	} else {
		status.VaultReachable = true
		status.VaultInitialized = vaultStatus.Initialized
		status.VaultSealed = vaultStatus.Sealed
		status.Phase = "initialize-vault"
		if vaultStatus.Initialized {
			status.Phase = "unseal-vault"
		}
		if vaultStatus.Initialized && !vaultStatus.Sealed {
			status.Phase = "configure-vault"
		}
	}
	if _, err := s.k8s.CoreV1().Secrets(s.namespace).Get(ctx, initSecretName, metav1.GetOptions{}); err == nil {
		status.RecoverySecretFound = true
	}
	if state, err := s.k8s.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{}); err == nil && state.Data["phase"] != "" {
		status.Phase = state.Data["phase"]
		status.ButlerLoginVerified = state.Data["butlerLoginVerified"] == "true"
		status.VaultLoginVerified = state.Data["vaultLoginVerified"] == "true"
	}
	return status
}

func (s *Service) Advance(ctx context.Context, confirmed bool) error {
	if !confirmed {
		return fmt.Errorf("explicit bootstrap confirmation is required")
	}
	status := s.Status(ctx)
	if status.Phase == "operational" {
		return nil
	}
	if status.VaultInitialized && !status.RecoverySecretFound {
		return fmt.Errorf("Vault is initialized but the recovery Secret is absent; refusing to continue")
	}
	if err := s.bootstrapper.Reconcile(ctx); err != nil {
		return err
	}
	configured, err := s.vault.SecretExists(ctx, "pocket-id/admin")
	if err != nil {
		return fmt.Errorf("checking Pocket ID identity handoff: %w", err)
	}
	if !configured {
		return s.setPhase(ctx, "awaiting-pocket-id-api-key")
	}
	if s.identityBootstrapper == nil {
		return fmt.Errorf("Pocket ID identity bootstrap is not configured")
	}
	if err := s.setPhase(ctx, "configure-identity"); err != nil {
		return err
	}
	if err := s.identityBootstrapper.Reconcile(ctx); err != nil {
		return fmt.Errorf("configuring Pocket ID identity: %w", err)
	}
	// Re-run the Vault foundation after Pocket ID clients exist so Vault's
	// OIDC method is configured with the newly generated client credential.
	if err := s.bootstrapper.Reconcile(ctx); err != nil {
		return fmt.Errorf("completing Vault OIDC handoff: %w", err)
	}
	return s.setPhase(ctx, "awaiting-identity-verification")
}

type IdentityEvidence struct {
	PocketIDSubject string   `json:"pocketIdSubject"`
	ButlerRole      string   `json:"butlerRole"`
	VaultPolicies   []string `json:"vaultPolicies"`
}

// ConfirmIdentity records only non-secret evidence after homelabctl has
// completed a real Pocket ID login to Butler and a separate Vault OIDC login.
// The Vault token is verified and revoked on the workstation and never crosses
// this API boundary.
func (s *Service) ConfirmIdentity(ctx context.Context, evidence IdentityEvidence) error {
	if strings.TrimSpace(evidence.PocketIDSubject) == "" || evidence.ButlerRole != "admin" {
		return fmt.Errorf("a Pocket ID-authenticated Butler administrator is required")
	}
	if !contains(evidence.VaultPolicies, "vault-admin") || !contains(evidence.VaultPolicies, "k8s-admin") {
		return fmt.Errorf("Vault OIDC login did not receive the required administrator policies")
	}
	state, err := s.k8s.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("reading bootstrap identity phase: %w", err)
	}
	if state.Data["phase"] != "awaiting-identity-verification" && state.Data["phase"] != "operational" {
		return fmt.Errorf("identity verification is not available during phase %q", state.Data["phase"])
	}
	if state.Data == nil {
		state.Data = map[string]string{}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	state.Data["butlerLoginVerified"] = "true"
	state.Data["vaultLoginVerified"] = "true"
	state.Data["identitySubject"] = evidence.PocketIDSubject
	state.Data["identityVerifiedAt"] = now
	state.Data["phase"] = "operational"
	state.Data["updatedAt"] = now
	state.Data["completedAt"] = now
	if _, err := s.k8s.CoreV1().ConfigMaps(s.namespace).Update(ctx, state, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("recording identity verification: %w", err)
	}
	return nil
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (s *Service) setPhase(ctx context.Context, phase string) error {
	state, err := s.k8s.CoreV1().ConfigMaps(s.namespace).Get(ctx, stateConfigMapName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		state = &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: stateConfigMapName, Namespace: s.namespace}}
		state.Data = map[string]string{"phase": phase, "updatedAt": time.Now().UTC().Format(time.RFC3339)}
		if _, createErr := s.k8s.CoreV1().ConfigMaps(s.namespace).Create(ctx, state, metav1.CreateOptions{}); createErr != nil {
			return fmt.Errorf("recording bootstrap phase: %w", createErr)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading bootstrap completion: %w", err)
	}
	if state.Data == nil {
		state.Data = map[string]string{}
	}
	state.Data["phase"] = phase
	state.Data["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	if phase == "operational" {
		state.Data["completedAt"] = time.Now().UTC().Format(time.RFC3339)
	}
	if _, err := s.k8s.CoreV1().ConfigMaps(s.namespace).Update(ctx, state, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating bootstrap phase: %w", err)
	}
	return nil
}
