package cli

import (
	"context"
	"fmt"
	"regexp"
	"runtime"
	"strings"

	"github.com/Masterminds/semver/v3"
	selfupdate "github.com/creativeprojects/go-selfupdate"
	"github.com/spf13/cobra"
)

const (
	releaseOwner      = "iamkhattar"
	releaseRepository = "homelab"
)

var completeSemanticVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

type availableRelease struct {
	version string
	url     string
	native  *selfupdate.Release
}

type selfUpdateClient interface {
	detect(context.Context, string) (availableRelease, bool, error)
	install(context.Context, availableRelease, string) error
}

type selfUpdateDependencies struct {
	newClient      func() (selfUpdateClient, error)
	executablePath func() (string, error)
	goos           string
	goarch         string
}

type githubSelfUpdateClient struct {
	updater    *selfupdate.Updater
	repository selfupdate.Repository
}

func productionSelfUpdateDependencies() selfUpdateDependencies {
	return selfUpdateDependencies{
		newClient:      newGitHubSelfUpdateClient,
		executablePath: selfupdate.ExecutablePath,
		goos:           runtime.GOOS,
		goarch:         runtime.GOARCH,
	}
}

func newGitHubSelfUpdateClient() (selfUpdateClient, error) {
	source, err := selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
	if err != nil {
		return nil, fmt.Errorf("configuring GitHub release source: %w", err)
	}
	updater, err := selfupdate.NewUpdater(selfupdate.Config{
		Source: source,
		Validator: &selfupdate.ChecksumValidator{
			UniqueFilename: "checksums.txt",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("configuring release updater: %w", err)
	}
	return &githubSelfUpdateClient{
		updater:    updater,
		repository: selfupdate.NewRepositorySlug(releaseOwner, releaseRepository),
	}, nil
}

func (c *githubSelfUpdateClient) detect(ctx context.Context, version string) (availableRelease, bool, error) {
	var (
		release *selfupdate.Release
		found   bool
		err     error
	)
	if version == "" {
		release, found, err = c.updater.DetectLatest(ctx, c.repository)
	} else {
		release, found, err = c.updater.DetectVersion(ctx, c.repository, version)
	}
	if err != nil || !found {
		return availableRelease{}, found, err
	}
	return availableRelease{version: release.Version(), url: release.URL, native: release}, true, nil
}

func (c *githubSelfUpdateClient) install(ctx context.Context, release availableRelease, path string) error {
	if release.native == nil {
		return fmt.Errorf("release has no downloadable asset")
	}
	return c.updater.UpdateTo(ctx, release.native, path)
}

func newSelfUpdateCommand(s *state, dependencies selfUpdateDependencies) *cobra.Command {
	var checkOnly bool
	var requestedVersion string
	var force bool

	cmd := &cobra.Command{
		Use:   "self-update",
		Short: "Update homelabctl from a verified GitHub Release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !supportedReleasePlatform(dependencies.goos, dependencies.goarch) {
				return fmt.Errorf("self-update is not supported on %s/%s; supported platforms are linux and darwin on amd64 and arm64", dependencies.goos, dependencies.goarch)
			}

			version, err := normalizeRequestedVersion(requestedVersion)
			if err != nil {
				return err
			}
			client, err := dependencies.newClient()
			if err != nil {
				return err
			}
			release, found, err := client.detect(cmd.Context(), version)
			if err != nil {
				return fmt.Errorf("checking GitHub releases: %w", err)
			}
			if !found {
				if version == "" {
					return fmt.Errorf("no compatible homelabctl release found for %s/%s", dependencies.goos, dependencies.goarch)
				}
				return fmt.Errorf("homelabctl release v%s was not found for %s/%s", version, dependencies.goos, dependencies.goarch)
			}

			current := strings.TrimPrefix(strings.TrimSpace(s.build.Version), "v")
			s.print("Current version: %s\n", displayVersion(current))
			s.print("Target version:  v%s\n", release.version)
			if release.url != "" {
				s.print("Release:         %s\n", release.url)
			}

			sameVersion := semanticVersionsEqual(current, release.version)
			if checkOnly || s.dryRun {
				if sameVersion {
					s.print("Status:          up to date\n")
				} else {
					s.print("Status:          update available\n")
				}
				return nil
			}
			if sameVersion && !force {
				s.print("homelabctl is already up to date; use --force to reinstall it.\n")
				return nil
			}

			path, err := dependencies.executablePath()
			if err != nil {
				return fmt.Errorf("locating the running homelabctl executable: %w", err)
			}
			if err := client.install(cmd.Context(), release, path); err != nil {
				return fmt.Errorf("installing homelabctl v%s at %s: %w (if the binary is root-owned, rerun with sudo)", release.version, path, err)
			}
			s.print("Updated %s to homelabctl v%s.\n", path, release.version)
			return nil
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "report whether an update is available without replacing the binary")
	cmd.Flags().StringVar(&requestedVersion, "version", "", "install an exact semantic version instead of the latest release")
	cmd.Flags().BoolVar(&force, "force", false, "reinstall when the target version is already running")
	return cmd
}

func supportedReleasePlatform(goos, goarch string) bool {
	return (goos == "linux" || goos == "darwin") && (goarch == "amd64" || goarch == "arm64")
}

func normalizeRequestedVersion(version string) (string, error) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if version == "" {
		return "", nil
	}
	if !completeSemanticVersion.MatchString(version) {
		return "", fmt.Errorf("version must be a complete semantic version such as v0.1.42")
	}
	parsed, err := semver.NewVersion(version)
	if err != nil {
		return "", fmt.Errorf("version must be a complete semantic version such as v0.1.42")
	}
	return parsed.String(), nil
}

func semanticVersionsEqual(left, right string) bool {
	leftVersion, leftErr := semver.NewVersion(left)
	rightVersion, rightErr := semver.NewVersion(right)
	return leftErr == nil && rightErr == nil && leftVersion.Equal(rightVersion)
}

func displayVersion(version string) string {
	if _, err := semver.NewVersion(version); err == nil {
		return "v" + version
	}
	if version == "" {
		return "dev"
	}
	return version
}
