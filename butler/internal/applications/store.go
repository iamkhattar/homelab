// Package applications owns Butler-managed application integration metadata.
package applications

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const configMapName = "butler-application-integrations"

var safeName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)

type Integration struct {
	Name           string   `json:"name"`
	Namespace      string   `json:"namespace"`
	Authentication string   `json:"authentication"`
	VaultPaths     []string `json:"vaultPaths,omitempty"`
	IngressHost    string   `json:"ingressHost,omitempty"`
	Owner          string   `json:"owner"`
}

type Store struct {
	k8s       kubernetes.Interface
	namespace string
}

func NewStore(k8s kubernetes.Interface, namespace string) *Store {
	return &Store{k8s: k8s, namespace: namespace}
}

func (s *Store) List(ctx context.Context) ([]Integration, error) {
	items, _, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Integration, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Store) Put(ctx context.Context, integration Integration) error {
	if err := validate(integration); err != nil {
		return err
	}
	items, resourceVersion, err := s.load(ctx)
	if err != nil {
		return err
	}
	items[integration.Name] = integration
	raw, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("encoding application integrations: %w", err)
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: configMapName, Namespace: s.namespace, ResourceVersion: resourceVersion},
		Data:       map[string]string{"integrations.json": string(raw)},
	}
	if resourceVersion == "" {
		_, err = s.k8s.CoreV1().ConfigMaps(s.namespace).Create(ctx, configMap, metav1.CreateOptions{})
	} else {
		_, err = s.k8s.CoreV1().ConfigMaps(s.namespace).Update(ctx, configMap, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("storing application integrations: %w", err)
	}
	return nil
}

func (s *Store) load(ctx context.Context) (map[string]Integration, string, error) {
	configMap, err := s.k8s.CoreV1().ConfigMaps(s.namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return map[string]Integration{}, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("reading application integrations: %w", err)
	}
	items := map[string]Integration{}
	if raw := configMap.Data["integrations.json"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &items); err != nil {
			return nil, "", fmt.Errorf("decoding application integrations: %w", err)
		}
	}
	return items, configMap.ResourceVersion, nil
}

func validate(integration Integration) error {
	if !safeName.MatchString(integration.Name) || !safeName.MatchString(integration.Namespace) {
		return fmt.Errorf("name and namespace must be DNS labels")
	}
	switch integration.Authentication {
	case "native-oidc", "forward-auth", "none":
	default:
		return fmt.Errorf("authentication must be native-oidc, forward-auth, or none")
	}
	if strings.TrimSpace(integration.Owner) == "" {
		return fmt.Errorf("owner is required")
	}
	for _, path := range integration.VaultPaths {
		if strings.TrimSpace(path) == "" || strings.Contains(path, "..") {
			return fmt.Errorf("invalid Vault path")
		}
	}
	return nil
}
