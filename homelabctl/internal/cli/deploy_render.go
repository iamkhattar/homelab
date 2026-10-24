package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/iamkhattar/homelab/homelabctl/internal/repository"
)

// renderedObject is the part of a rendered manifest the network summary needs.
type renderedObject struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
	Spec struct {
		PodSelector *map[string]any `yaml:"podSelector"`
	} `yaml:"spec"`
}

// namespaceValues is the declared namespace list.
type namespaceValues struct {
	Namespaces []struct {
		Name string `yaml:"name"`
	} `yaml:"namespaces"`
}

// newDeployRenderCommand renders the whole platform offline. It never
// contacts a cluster, so an operator can review the desired state before
// reaching the LAN.
func newDeployRenderCommand(s *state, imageTag *string) *cobra.Command {
	var outputPath string
	var requireBaselines bool
	command := &cobra.Command{
		Use:   "render",
		Short: "Render the platform manifests offline for review",
		Long: "Render every enabled release from the Helmfile without contacting a cluster.\n" +
			"The render reports the object and namespace inventory and whether each declared\n" +
			"namespace carries a namespace-wide NetworkPolicy baseline.",
		Example: "  homelabctl deploy render\n  homelabctl deploy render --output /tmp/platform.yaml\n  homelabctl deploy render --require-network-baselines",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			manifests, err := renderPlatform(cmd.Context(), s, *imageTag)
			if err != nil {
				return err
			}
			declared, err := declaredNamespaces(s)
			if err != nil {
				return err
			}
			summary, missing := summariseNetworkPolicies(manifests, declared)

			if outputPath == "" {
				if _, err := fmt.Fprint(s.out(), manifests); err != nil {
					return fmt.Errorf("writing rendered manifests: %w", err)
				}
			} else {
				if err := os.WriteFile(outputPath, []byte(manifests), 0o600); err != nil {
					return fmt.Errorf("writing rendered manifests: %w", err)
				}
				s.success(fmt.Sprintf("Wrote rendered manifests to %s", outputPath))
			}

			s.keyValue("Declared namespaces", fmt.Sprintf("%d", len(declared)))
			s.keyValue("NetworkPolicy objects", fmt.Sprintf("%d", summary.policies))
			s.keyValue("Namespace-wide baselines", fmt.Sprintf("%d", summary.baselines))
			if len(missing) > 0 {
				s.warning(fmt.Sprintf("Namespaces without a namespace-wide baseline: %s", strings.Join(missing, ", ")))
			}
			if requireBaselines && len(missing) > 0 {
				return fmt.Errorf("namespaces without a namespace-wide NetworkPolicy baseline: %s", strings.Join(missing, ", "))
			}
			return nil
		},
	}
	command.Flags().StringVar(&outputPath, "output", "", "write the rendered manifests to this file instead of standard output")
	command.Flags().BoolVar(&requireBaselines, "require-network-baselines", false, "fail when a declared namespace has no namespace-wide NetworkPolicy baseline")
	return command
}

// renderPlatform renders every enabled release offline.
func renderPlatform(ctx context.Context, s *state, imageTag string) (string, error) {
	if imageTag == "" {
		sha, err := repository.HeadSHA(s.root)
		if err != nil {
			return "", fmt.Errorf("resolving default render image tag from Git: %w", err)
		}
		imageTag = sha
	}
	if err := validateTags([]string{imageTag}); err != nil {
		return "", err
	}
	manifests, err := s.outputEnv(ctx, s.dir("cluster"), map[string]string{"HOMELAB_IMAGE_TAG": imageTag}, "helmfile", "template", "--skip-deps")
	if err != nil {
		return "", fmt.Errorf("rendering the platform: %w", err)
	}
	return manifests, nil
}

// declaredNamespaces reads the declared namespace list from the repository.
func declaredNamespaces(s *state) ([]string, error) {
	raw, err := os.ReadFile(s.dir("cluster", "core", "namespaces", "values.yaml"))
	if err != nil {
		return nil, fmt.Errorf("reading declared namespaces: %w", err)
	}
	var values namespaceValues
	if err := yaml.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("parsing declared namespaces: %w", err)
	}
	names := make([]string, 0, len(values.Namespaces))
	for _, namespace := range values.Namespaces {
		if namespace.Name != "" {
			names = append(names, namespace.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// networkSummary counts the rendered policies and namespace-wide baselines.
type networkSummary struct {
	policies  int
	baselines int
}

// summariseNetworkPolicies reports the rendered NetworkPolicy inventory and
// returns the declared namespaces that carry no namespace-wide baseline.
func summariseNetworkPolicies(manifests string, declared []string) (networkSummary, []string) {
	summary := networkSummary{}
	baselines := map[string]bool{}
	decoder := yaml.NewDecoder(strings.NewReader(manifests))
	for {
		var object renderedObject
		if err := decoder.Decode(&object); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			break
		}
		if object.Kind != "NetworkPolicy" {
			continue
		}
		summary.policies++
		if object.Spec.PodSelector != nil && len(*object.Spec.PodSelector) == 0 {
			summary.baselines++
			baselines[object.Metadata.Namespace] = true
		}
	}
	missing := []string{}
	for _, namespace := range declared {
		if !baselines[namespace] {
			missing = append(missing, namespace)
		}
	}
	return summary, missing
}
