package reconciler

import (
	"context"
	"fmt"

	"github.com/iamkhattar/homelab/butler/internal/config"
	"github.com/iamkhattar/homelab/butler/internal/garage"
	"github.com/iamkhattar/homelab/butler/internal/vault"
)

type Garage struct {
	vault *vault.Client
	cfg   config.GarageConfig
}

func NewGarage(vc *vault.Client, cfg config.GarageConfig) *Garage {
	return &Garage{vault: vc, cfg: cfg}
}
func (r *Garage) Name() string { return "garage" }

func (r *Garage) Reconcile(ctx context.Context) error {
	if !r.cfg.Enabled {
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
	for _, bucket := range r.cfg.Buckets {
		if err := r.ensureBucket(ctx, client, bucket); err != nil {
			return err
		}
	}
	return nil
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

func (r *Garage) ensureBucket(ctx context.Context, client *garage.Client, spec config.GarageBucketSpec) error {
	bucket, err := client.Bucket(ctx, spec.Name)
	if err != nil {
		return err
	}
	if bucket == nil {
		bucket, err = client.CreateBucket(ctx, spec.Name)
		if err != nil {
			return err
		}
	}
	creds, err := r.vault.ReadSecretIfExists(ctx, spec.CredentialPath)
	if err != nil {
		return err
	}
	keyID, _ := creds["access-key-id"].(string)
	if keyID == "" {
		key, err := client.CreateKey(ctx, spec.Name)
		if err != nil {
			return err
		}
		keyID = key.AccessKeyID
		creds = map[string]interface{}{"access-key-id": key.AccessKeyID, "secret-access-key": key.SecretAccessKey, "endpoint": "http://garage.storage.svc.cluster.local:3900", "region": "garage", "bucket": spec.Name}
		if err := r.vault.WriteSecret(ctx, spec.CredentialPath, creds); err != nil {
			return err
		}
	}
	return client.AllowBucketKey(ctx, bucket.ID, keyID, spec.Read, spec.Write, spec.Owner)
}
