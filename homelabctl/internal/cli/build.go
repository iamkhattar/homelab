package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/iamkhattar/homelab/homelabctl/internal/repository"
)

type serviceBuildOptions struct {
	tags     []string
	registry string
	push     bool
	changed  bool
	base     string
}

type docsBuildOptions struct {
	tags  []string
	image string
	push  bool
}

func newBuildCommand(s *state) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build repository artifacts",
	}

	var tags []string
	var registry string
	var push bool
	var changed bool
	var base string
	servicesCmd := &cobra.Command{
		Use:   "services [service...]",
		Short: "Build service container images",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, names []string) error {
			return buildServices(cmd, s, names, serviceBuildOptions{
				tags: tags, registry: registry, push: push, changed: changed, base: base,
			})
		},
	}
	servicesCmd.Flags().StringSliceVar(&tags, "tag", nil, "image tag; repeat for multiple tags (default: current Git SHA)")
	servicesCmd.Flags().StringVar(&registry, "registry", "iamkhattar", "container registry namespace")
	servicesCmd.Flags().BoolVar(&push, "push", false, "push built images (requires CI)")
	servicesCmd.Flags().BoolVar(&changed, "changed", false, "build only services changed from --base")
	servicesCmd.Flags().StringVar(&base, "base", "", "Git base revision used with --changed")

	var docsTags []string
	var docsImage string
	var docsPush bool
	docsCmd := &cobra.Command{
		Use:   "docs",
		Short: "Build the VitePress Nginx image",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return buildDocs(cmd.Context(), s, docsBuildOptions{tags: docsTags, image: docsImage, push: docsPush})
		},
	}
	docsCmd.Flags().StringSliceVar(&docsTags, "tag", nil, "image tag; repeat for multiple tags (default: current Git SHA)")
	docsCmd.Flags().StringVar(&docsImage, "image", "iamkhattar/homelab-docs", "container image name")
	docsCmd.Flags().BoolVar(&docsPush, "push", false, "push the built image (requires CI)")

	cmd.AddCommand(servicesCmd, docsCmd)
	return cmd
}

func buildServices(cmd *cobra.Command, s *state, names []string, options serviceBuildOptions) error {
	if options.push && os.Getenv("CI") == "" {
		return fmt.Errorf("--push is only allowed when CI is set")
	}
	if options.changed && len(names) > 0 {
		return fmt.Errorf("--changed cannot be combined with explicit service names")
	}
	if err := validateContainerValue(options.registry, "registry namespace"); err != nil {
		return err
	}
	if err := validateTags(options.tags); err != nil {
		return err
	}
	if options.changed && strings.TrimSpace(options.base) == "" {
		return fmt.Errorf("--base is required with --changed")
	}
	resolvedTags, err := resolveImageTags(cmd.Context(), s, options.tags)
	if err != nil {
		return err
	}
	options.tags = resolvedTags

	services, err := repository.Services(s.root)
	if err != nil {
		return err
	}
	if options.changed {
		names, err = changedServiceNames(s, options.base)
		if err != nil {
			return err
		}
	}

	selected, err := selectServices(services, names)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		s.info("no service images need to be built")
		return nil
	}

	for _, service := range selected {
		args := []string{"build"}
		var images []string
		for _, tag := range options.tags {
			image := fmt.Sprintf("%s/%s:%s", options.registry, service.Name, tag)
			args = append(args, "--tag", image)
			images = append(images, image)
		}
		args = append(args, ".")
		if err := s.run(cmd.Context(), service.Dir, "docker", args...); err != nil {
			return err
		}
		if options.push {
			for _, image := range images {
				if err := s.run(cmd.Context(), service.Dir, "docker", "push", image); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func buildDocs(ctx context.Context, s *state, options docsBuildOptions) error {
	if options.push && os.Getenv("CI") == "" {
		return fmt.Errorf("--push is only allowed when CI is set")
	}
	if err := validateContainerValue(options.image, "documentation image"); err != nil {
		return err
	}
	if err := validateTags(options.tags); err != nil {
		return err
	}
	resolvedTags, err := resolveImageTags(ctx, s, options.tags)
	if err != nil {
		return err
	}
	options.tags = resolvedTags

	args := []string{"build"}
	var images []string
	for _, tag := range options.tags {
		image := options.image + ":" + tag
		args = append(args, "--tag", image)
		images = append(images, image)
	}
	args = append(args, "docs")
	if err := s.run(ctx, s.root, "docker", args...); err != nil {
		return err
	}
	if options.push {
		for _, image := range images {
			if err := s.run(ctx, s.root, "docker", "push", image); err != nil {
				return err
			}
		}
	}
	return nil
}

func resolveImageTags(ctx context.Context, s *state, tags []string) ([]string, error) {
	if len(tags) > 0 {
		if err := validateTags(tags); err != nil {
			return nil, err
		}
		return tags, nil
	}

	sha, err := repository.HeadSHA(s.root)
	if err != nil {
		return nil, fmt.Errorf("resolving default image tag from Git: %w", err)
	}
	if !gitSHAImageTagPattern.MatchString(sha) {
		return nil, fmt.Errorf("Git returned an invalid commit SHA for the default image tag")
	}
	return []string{sha}, nil
}

func changedServiceNames(s *state, base string) ([]string, error) {
	return repository.ChangedServiceNames(s.root, base)
}

func selectServices(services []repository.Service, names []string) ([]repository.Service, error) {
	if len(names) == 0 {
		return services, nil
	}
	wanted := make(map[string]struct{}, len(names))
	for _, name := range names {
		wanted[name] = struct{}{}
	}
	var selected []repository.Service
	for _, service := range services {
		if _, ok := wanted[service.Name]; ok {
			selected = append(selected, service)
			delete(wanted, service.Name)
		}
	}
	if len(wanted) > 0 {
		var unknown []string
		for name := range wanted {
			unknown = append(unknown, name)
		}
		sort.Strings(unknown)
		return nil, fmt.Errorf("unknown service(s): %s", strings.Join(unknown, ", "))
	}
	return selected, nil
}
