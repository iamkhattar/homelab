package control

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// KubernetesCredential is the dynamic secret Vault's kubernetes/ secrets engine
// returns. It is short-lived and audience-bound to the cluster.
type KubernetesCredential struct {
	ServiceAccountName      string
	ServiceAccountNamespace string
	Token                   string
	LeaseID                 string
	LeaseDuration           time.Duration
}

// credentialResponse mirrors the fields Vault returns from
// kubernetes/creds/<role>. Only the documented keys are read.
type credentialResponse struct {
	Data struct {
		ServiceAccountName      string `json:"service_account_name"`
		ServiceAccountNamespace string `json:"service_account_namespace"`
		Token                   string `json:"service_account_token"`
		LeaseID                 string `json:"lease_id"`
		LeaseDuration           int    `json:"lease_duration"`
	} `json:"data"`
	Errors []string `json:"errors"`
}

// ReadKubernetesCredential asks Vault to mint a Kubernetes credential as the
// authenticated identity. Authorization is entirely the group-bound Vault
// policy on the token: when the group does not carry the role, Vault's own
// permission error is returned unchanged so the operator sees the real denial.
func ReadKubernetesCredential(ctx context.Context, client *http.Client, vaultAddress, vaultToken, role, namespace, ttl string) (KubernetesCredential, error) {
	if vaultAddress == "" || vaultToken == "" {
		return KubernetesCredential{}, fmt.Errorf("Vault address and token are required")
	}
	if role == "" || namespace == "" {
		return KubernetesCredential{}, fmt.Errorf("Vault role and Kubernetes namespace are required")
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	endpoint := strings.TrimRight(vaultAddress, "/") + "/v1/kubernetes/creds/" + url.PathEscape(role)
	var response credentialResponse
	body := map[string]string{"kubernetes_namespace": namespace, "ttl": ttl}
	if err := vaultJSON(ctx, client, http.MethodPost, endpoint, vaultToken, body, &response); err != nil {
		return KubernetesCredential{}, err
	}
	if response.Data.Token == "" {
		return KubernetesCredential{}, fmt.Errorf("Vault returned no Kubernetes token for role %q", role)
	}
	return KubernetesCredential{
		ServiceAccountName:      response.Data.ServiceAccountName,
		ServiceAccountNamespace: response.Data.ServiceAccountNamespace,
		Token:                   response.Data.Token,
		LeaseID:                 response.Data.LeaseID,
		LeaseDuration:           time.Duration(response.Data.LeaseDuration) * time.Second,
	}, nil
}
