package vault

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	butlercrypto "github.com/iamkhattar/homelab/butler/internal/crypto"
)

// SecretExists checks whether a secret exists at the given KV-v2 path.
func (c *Client) SecretExists(ctx context.Context, path string) (bool, error) {
	secret, err := c.raw.KVv2("secret").Get(ctx, path)
	if err != nil {
		// Vault returns 404 as an error for missing secrets.
		if isNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading secret at %s: %w", path, err)
	}
	return secret != nil && secret.Data != nil, nil
}

// ReadSecret reads all key-value pairs at the given KV-v2 path.
func (c *Client) ReadSecret(ctx context.Context, path string) (map[string]interface{}, error) {
	secret, err := c.raw.KVv2("secret").Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("reading secret at %s: %w", path, err)
	}
	if secret == nil || secret.Data == nil {
		return nil, nil
	}
	return secret.Data, nil
}

// ReadSecretIfExists returns an empty map when the KV path does not exist.
func (c *Client) ReadSecretIfExists(ctx context.Context, path string) (map[string]interface{}, error) {
	exists, err := c.SecretExists(ctx, path)
	if err != nil || !exists {
		return map[string]interface{}{}, err
	}
	return c.ReadSecret(ctx, path)
}

// WriteSecret writes key-value pairs to the given KV-v2 path.
func (c *Client) WriteSecret(ctx context.Context, path string, data map[string]interface{}) error {
	if _, err := c.raw.KVv2("secret").Put(ctx, path, data); err != nil {
		return fmt.Errorf("writing secret at %s: %w", path, err)
	}
	slog.Info("wrote secret", "path", path)
	return nil
}

// GenerateSecretData creates a map of key-value pairs based on the provided
// key configurations. For template values, it resolves references to other
// keys within the same secret (e.g. {{db-password}}).
func GenerateSecretData(keys map[string]KeySpec) (map[string]interface{}, error) {
	return GenerateSecretDataWithExisting(keys, nil)
}

// GenerateSecretDataWithExisting fills only missing keys and preserves every
// existing value. This lets Butler extend a managed credential definition
// without rotating credentials that consumers already use.
func GenerateSecretDataWithExisting(keys map[string]KeySpec, existing map[string]interface{}) (map[string]interface{}, error) {
	data := make(map[string]interface{}, len(existing)+len(keys))
	for name, value := range existing {
		data[name] = value
	}
	for name, spec := range keys {
		methods := 0
		for _, configured := range []bool{spec.Length > 0, spec.HexLength > 0, spec.Static != "", spec.Template != ""} {
			if configured {
				methods++
			}
		}
		if methods != 1 {
			return nil, fmt.Errorf("key %s must set exactly one of length, hexLength, static, or template", name)
		}
	}

	// First pass: generate all non-template values.
	for name, spec := range keys {
		if _, ok := data[name]; ok {
			continue
		}
		if spec.Template != "" {
			continue
		}
		if spec.Static != "" {
			data[name] = spec.Static
			continue
		}
		if spec.Length > 0 {
			pw, err := butlercrypto.GeneratePassword(spec.Length)
			if err != nil {
				return nil, fmt.Errorf("generating value for key %s: %w", name, err)
			}
			data[name] = pw
			continue
		}
		if spec.HexLength > 0 {
			value, err := butlercrypto.GenerateHex(spec.HexLength)
			if err != nil {
				return nil, fmt.Errorf("generating hex value for key %s: %w", name, err)
			}
			data[name] = value
			continue
		}
		return nil, fmt.Errorf("key %s has no supported generator", name)
	}

	// Second pass: resolve templates.
	for name, spec := range keys {
		if spec.Template == "" {
			continue
		}
		if _, ok := data[name]; ok {
			continue
		}
		resolved := spec.Template
		for k, v := range data {
			resolved = strings.ReplaceAll(resolved, "{{"+k+"}}", fmt.Sprintf("%v", v))
		}
		data[name] = resolved
	}

	return data, nil
}

// KeySpec describes a single secret key's generation parameters.
type KeySpec struct {
	Length    int
	HexLength int
	Static    string
	Template  string
}

// isNotFound returns true if the Vault error indicates a 404.
func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "secret not found")
}
