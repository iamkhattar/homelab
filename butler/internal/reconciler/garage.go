package reconciler

import (
	"context"
	"errors"
	"fmt"

	"github.com/iamkhattar/homelab/butler/internal/config"
	"github.com/iamkhattar/homelab/butler/internal/garage"
	"github.com/iamkhattar/homelab/butler/internal/platform"
	"github.com/iamkhattar/homelab/butler/internal/vault"
)

type Garage struct {
	vault     *vault.Client
	cfg       config.GarageConfig
	resources platform.Resources
}

func NewGarage(vc *vault.Client, cfg config.GarageConfig, resources platform.Resources) *Garage {
	return &Garage{vault: vc, cfg: cfg, resources: resources}
}
func (r *Garage) Name() string { return "garage" }

func (r *Garage) Reconcile(ctx context.Context) error {
	if !r.cfg.Enabled {
		return nil
	}
	buckets, err := r.resources.ListGarageBuckets(ctx)
	if err != nil {
		return fmt.Errorf("listing GarageBuckets: %w", err)
	}
	// Garage belongs to the later data stage. Until that chart declares a
	// bucket there is no provider state to reconcile and its admin credential
	// is intentionally absent.
	if len(buckets) == 0 {
		return nil
	}
	secret, err := r.vault.ReadSecret(ctx, r.cfg.AdminTokenPath)
	if err != nil {
		return err
	}
	token, _ := secret[r.cfg.AdminTokenKey].(string)
	if token == "" {
		return fmt.Errorf("garage admin token is missing at %s", r.cfg.AdminTokenPath)
	}
	client := garage.NewClient(r.cfg.Endpoint, token)
	status, err := client.ClusterStatus(ctx)
	if err != nil {
		return err
	}
	if err := r.ensureLayout(ctx, client, status); err != nil {
		return err
	}
	var failures []error
	bucketCounts := make(map[string]int, len(buckets))
	for i := range buckets {
		bucketCounts[buckets[i].Spec.BucketName]++
	}
	pathCounts, err := platform.VaultPathOwners(ctx, r.resources)
	if err != nil {
		return err
	}
	for i := range buckets {
		if bucketCounts[buckets[i].Spec.BucketName] > 1 {
			err := fmt.Errorf("Garage bucket name %q must be unique across namespaces", buckets[i].Spec.BucketName)
			failures = append(failures, err)
			if statusErr := convergeStatus(&buckets[i].Status, platform.Failed(buckets[i].Generation, "DuplicateProviderName", err), func() error {
				return r.resources.UpdateGarageBucketStatus(ctx, &buckets[i])
			}); statusErr != nil {
				failures = append(failures, statusErr)
			}
			continue
		}
		if pathCounts[buckets[i].Spec.CredentialPath] > 1 {
			err := fmt.Errorf("Vault path %q must be owned by exactly one platform resource", buckets[i].Spec.CredentialPath)
			failures = append(failures, err)
			if statusErr := convergeStatus(&buckets[i].Status, platform.Failed(buckets[i].Generation, "DuplicateVaultPath", err), func() error {
				return r.resources.UpdateGarageBucketStatus(ctx, &buckets[i])
			}); statusErr != nil {
				failures = append(failures, statusErr)
			}
			continue
		}
		providerID, reconcileErr := r.ensureBucket(ctx, client, buckets[i].Spec)
		var desired platform.ResourceStatus
		if reconcileErr != nil {
			desired = platform.Failed(buckets[i].Generation, "ReconcileFailed", reconcileErr)
			failures = append(failures, fmt.Errorf("%s/%s: %w", buckets[i].Namespace, buckets[i].Name, reconcileErr))
		} else {
			desired = platform.Ready(buckets[i].Generation, providerID)
		}
		if statusErr := convergeStatus(&buckets[i].Status, desired, func() error {
			return r.resources.UpdateGarageBucketStatus(ctx, &buckets[i])
		}); statusErr != nil {
			failures = append(failures, statusErr)
		}
	}
	return errors.Join(failures...)
}

func (r *Garage) ensureLayout(ctx context.Context, client *garage.Client, status garage.ClusterStatus) error {
	if r.cfg.Layout.Zone == "" || r.cfg.Layout.CapacityBytes <= 0 {
		return fmt.Errorf("garage layout requires zone and positive capacityBytes")
	}
	for _, node := range status.Nodes {
		if node.Role != nil {
			return nil
		}
	}
	if len(status.Nodes) != 1 || !status.Nodes[0].IsUp {
		return fmt.Errorf("garage single-node layout requires exactly one live node")
	}
	if err := client.AssignNode(ctx, status.Nodes[0].ID, r.cfg.Layout.Zone, r.cfg.Layout.CapacityBytes); err != nil {
		return err
	}
	return client.ApplyLayout(ctx, status.LayoutVersion+1)
}

func (r *Garage) ensureBucket(ctx context.Context, client *garage.Client, spec platform.GarageBucketSpec) (string, error) {
	bucket, err := client.Bucket(ctx, spec.BucketName)
	if err != nil {
		return "", err
	}
	if bucket == nil {
		bucket, err = client.CreateBucket(ctx, spec.BucketName)
		if err != nil {
			return "", err
		}
	}
	creds, err := r.vault.ReadSecretIfExists(ctx, spec.CredentialPath)
	if err != nil {
		return "", err
	}
	keyID, _ := creds["access-key-id"].(string)
	if keyID == "" {
		key, err := client.CreateKey(ctx, spec.BucketName)
		if err != nil {
			return "", err
		}
		keyID = key.AccessKeyID
		creds = map[string]interface{}{"access-key-id": key.AccessKeyID, "secret-access-key": key.SecretAccessKey, "endpoint": "http://garage.storage.svc.cluster.local:3900", "region": "garage", "bucket": spec.BucketName}
		if err := r.vault.WriteSecret(ctx, spec.CredentialPath, creds); err != nil {
			// Garage exposes the secret only once. Revoke the just-created key if
			// Vault cannot durably accept it so no unknown credential survives.
			return "", errors.Join(err, client.DeleteKey(ctx, key.AccessKeyID))
		}
	}
	err = client.AllowBucketKey(ctx, bucket.ID, keyID, spec.Permissions.Read, spec.Permissions.Write, spec.Permissions.Owner)
	return bucket.ID, err
}
