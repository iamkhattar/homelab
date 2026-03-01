package ci

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/iamkhattar/homelab/hl/internal/lint"
	"github.com/iamkhattar/homelab/hl/internal/ui"
)

// tool describes an external dependency that hl ci commands need.
type tool struct {
	name    string
	version string
	// url returns the download URL for the given os/arch.
	url func(goos, goarch string) string
	// installed returns true if the tool is already on PATH.
	installed func() bool
	// postInstall is called after the binary is downloaded (e.g. untar).
	// If nil, the downloaded file is assumed to be the binary itself.
	postInstall func(downloadPath, binDir string) error
}

var tools = []tool{
	{
		name:    "helm",
		version: "4.1.1",
		url: func(goos, goarch string) string {
			return fmt.Sprintf("https://get.helm.sh/helm-v4.1.1-%s-%s.tar.gz", goos, goarch)
		},
		installed: func() bool { _, err := exec.LookPath("helm"); return err == nil },
		postInstall: func(downloadPath, binDir string) error {
			// helm tarball contains <os>-<arch>/helm — extract it.
			c := exec.Command("tar", "-xzf", downloadPath, "-C", binDir, "--strip-components=1",
				fmt.Sprintf("%s-%s/helm", runtime.GOOS, runtime.GOARCH))
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return err
			}
			return os.Remove(downloadPath)
		},
	},
	{
		name:    "helmfile",
		version: "1.3.2",
		url: func(goos, goarch string) string {
			return fmt.Sprintf("https://github.com/helmfile/helmfile/releases/download/v1.3.2/helmfile_1.3.2_%s_%s.tar.gz", goos, goarch)
		},
		installed: func() bool { _, err := exec.LookPath("helmfile"); return err == nil },
		postInstall: func(downloadPath, binDir string) error {
			c := exec.Command("tar", "-xzf", downloadPath, "-C", binDir, "helmfile")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return err
			}
			return os.Remove(downloadPath)
		},
	},
	{
		name:    "terraform",
		version: "1.14.5",
		url: func(goos, goarch string) string {
			return fmt.Sprintf("https://releases.hashicorp.com/terraform/1.14.5/terraform_1.14.5_%s_%s.zip", goos, goarch)
		},
		installed: func() bool { _, err := exec.LookPath("terraform"); return err == nil },
		postInstall: func(downloadPath, binDir string) error {
			c := exec.Command("unzip", "-o", downloadPath, "terraform", "-d", binDir)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return err
			}
			return os.Remove(downloadPath)
		},
	},
	{
		name:    "golangci-lint",
		version: "2.10.1",
		url: func(goos, goarch string) string {
			return fmt.Sprintf("https://github.com/golangci/golangci-lint/releases/download/v2.10.1/golangci-lint-2.10.1-%s-%s.tar.gz", goos, goarch)
		},
		installed: func() bool { _, err := exec.LookPath("golangci-lint"); return err == nil },
		postInstall: func(downloadPath, binDir string) error {
			c := exec.Command("tar", "-xzf", downloadPath, "-C", binDir, "--strip-components=1",
				fmt.Sprintf("golangci-lint-2.10.1-%s-%s/golangci-lint", runtime.GOOS, runtime.GOARCH))
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return err
			}
			return os.Remove(downloadPath)
		},
	},
	{
		name:    "gitleaks",
		version: "8.30.0",
		url: func(goos, goarch string) string {
			// gitleaks uses x64/x32 instead of amd64/386.
			arch := goarch
			switch goarch {
			case "amd64":
				arch = "x64"
			case "386":
				arch = "x32"
			}
			return fmt.Sprintf("https://github.com/gitleaks/gitleaks/releases/download/v8.30.0/gitleaks_%s_%s_%s.tar.gz",
				"8.30.0", goos, arch)
		},
		installed: func() bool { _, err := exec.LookPath("gitleaks"); return err == nil },
		postInstall: func(downloadPath, binDir string) error {
			c := exec.Command("tar", "-xzf", downloadPath, "-C", binDir, "gitleaks")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return err
			}
			return os.Remove(downloadPath)
		},
	},
	{
		name:    "syft",
		version: "1.29.0",
		url: func(goos, goarch string) string {
			return fmt.Sprintf("https://github.com/anchore/syft/releases/download/v1.29.0/syft_%s_%s_%s.tar.gz",
				"1.29.0", goos, goarch)
		},
		installed: func() bool { _, err := exec.LookPath("syft"); return err == nil },
		postInstall: func(downloadPath, binDir string) error {
			c := exec.Command("tar", "-xzf", downloadPath, "-C", binDir, "syft")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return err
			}
			return os.Remove(downloadPath)
		},
	},
	{
		name:    "gotestsum",
		version: "1.13.0",
		url: func(goos, goarch string) string {
return fmt.Sprintf("https://github.com/gotestyourself/gotestsum/releases/download/v1.13.0/gotestsum_%s_%s_%s.tar.gz",
				"1.13.0", goos, goarch)
		},
		installed: func() bool { _, err := exec.LookPath("gotestsum"); return err == nil },
		postInstall: func(downloadPath, binDir string) error {
			c := exec.Command("tar", "-xzf", downloadPath, "-C", binDir, "gotestsum")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return err
			}
			return os.Remove(downloadPath)
		},
	},
	{
		name:    "grype",
		version: "0.92.0",
		url: func(goos, goarch string) string {
			return fmt.Sprintf("https://github.com/anchore/grype/releases/download/v0.92.0/grype_%s_%s_%s.tar.gz",
				"0.92.0", goos, goarch)
		},
		installed: func() bool { _, err := exec.LookPath("grype"); return err == nil },
		postInstall: func(downloadPath, binDir string) error {
			c := exec.Command("tar", "-xzf", downloadPath, "-C", binDir, "grype")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return err
			}
			return os.Remove(downloadPath)
		},
	},
	{
		name:      "ansible-lint",
		version:   "25.4.0",
		installed: func() bool { _, err := exec.LookPath("ansible-lint"); return err == nil },
		postInstall: func(_, _ string) error {
			c := exec.Command("pip", "install", "ansible-lint==25.4.0")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	},
}

var setupCmd = &cobra.Command{
	Use:   "setup [tool...]",
	Short: "Install required tools for CI and lint",
	Long: `Download and install external tools needed by hl ci commands.

Tools: helm, helmfile, terraform, golangci-lint, gitleaks, gotestsum, syft, grype, ansible-lint

With no arguments, all tools are installed. Pass one or more tool names
to install only those tools.

Binaries are installed to ./bin (or $GITHUB_WORKSPACE/bin in CI).
Pip-based tools (e.g. ansible-lint) are installed via pip.
Already-installed tools are skipped.`,
	Example: `  hl ci setup                          # install all tools
  hl ci setup helm helmfile terraform   # install only these three
  hl ci setup golangci-lint             # install a single tool`,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var names []string
		for _, t := range tools {
			names = append(names, t.name)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		selected, err := selectTools(args)
		if err != nil {
			return err
		}

		binDir := setupBinDir()
		if err := os.MkdirAll(binDir, 0755); err != nil {
			return fmt.Errorf("creating bin dir: %w", err)
		}

		// Ensure binDir is on PATH for subsequent tool checks.
		if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
			return fmt.Errorf("setting PATH: %w", err)
		}

		ui.Step(fmt.Sprintf("Setting up tools in %s", binDir))

		goos := runtime.GOOS
		goarch := runtime.GOARCH

		for _, t := range selected {
			if t.installed() {
				ui.StepDone(fmt.Sprintf("%s %s (already installed)", t.name, t.version))
				continue
			}

			ui.Step(fmt.Sprintf("Installing %s %s", t.name, t.version))

			if t.url != nil {
				// Binary download flow.
				url := t.url(goos, goarch)
				downloadPath := filepath.Join(binDir, t.name+"-download")
				if err := downloadFile(url, downloadPath); err != nil {
					ui.StepFail(fmt.Sprintf("Failed to download %s", t.name))
					return fmt.Errorf("downloading %s: %w", t.name, err)
				}

				if t.postInstall != nil {
					if err := t.postInstall(downloadPath, binDir); err != nil {
						ui.StepFail(fmt.Sprintf("Failed to install %s", t.name))
						return fmt.Errorf("installing %s: %w", t.name, err)
					}
				}

				// Ensure the binary is executable.
				binPath := filepath.Join(binDir, t.name)
				if err := os.Chmod(binPath, 0755); err != nil {
					return err
				}
			} else if t.postInstall != nil {
				// Non-binary install (e.g. pip).
				if err := t.postInstall("", binDir); err != nil {
					ui.StepFail(fmt.Sprintf("Failed to install %s", t.name))
					return fmt.Errorf("installing %s: %w", t.name, err)
				}
			}

			ui.StepDone(fmt.Sprintf("%s %s", t.name, t.version))
		}

		fmt.Println()
		ui.StepDone(ui.SuccessStyle.Render("All tools ready"))
		return nil
	},
}

// selectTools returns the tools to install based on the given names.
// If names is empty, all tools are returned.
func selectTools(names []string) ([]tool, error) {
	if len(names) == 0 {
		return tools, nil
	}

	byName := make(map[string]tool, len(tools))
	for _, t := range tools {
		byName[t.name] = t
	}

	selected := make([]tool, 0, len(names))
	for _, name := range names {
		t, ok := byName[name]
		if !ok {
			var available []string
			for _, t := range tools {
				available = append(available, t.name)
			}
			return nil, fmt.Errorf("unknown tool %q (available: %v)", name, available)
		}
		selected = append(selected, t)
	}
	return selected, nil
}

// setupBinDir returns the directory where tools should be installed.
func setupBinDir() string {
	if ws := os.Getenv("GITHUB_WORKSPACE"); ws != "" {
		return filepath.Join(ws, "bin")
	}
	// Local dev: use repo-local .bin directory.
	root, err := lint.RepoRoot()
	if err != nil {
		return ".bin"
	}
	return filepath.Join(root, ".bin")
}

// downloadFile fetches a URL to a local file path.
func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = io.Copy(f, resp.Body)
	return err
}
