package build

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

// tool describes an external dependency that hl build commands need.
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
		version: "3.17.3",
		url: func(goos, goarch string) string {
			return fmt.Sprintf("https://get.helm.sh/helm-v3.17.3-%s-%s.tar.gz", goos, goarch)
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
		version: "0.171.0",
		url: func(goos, goarch string) string {
			return fmt.Sprintf("https://github.com/helmfile/helmfile/releases/download/v0.171.0/helmfile_%s_%s.tar.gz", goos, goarch)
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
		version: "1.11.4",
		url: func(goos, goarch string) string {
			return fmt.Sprintf("https://releases.hashicorp.com/terraform/1.11.4/terraform_1.11.4_%s_%s.zip", goos, goarch)
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
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Install required tools for build and lint",
	Long: `Download and install external tools needed by hl build commands.

Tools: helm, helmfile, terraform

Binaries are installed to ./bin (or $GITHUB_WORKSPACE/bin in CI).
Already-installed tools are skipped.`,
	Example: `  hl build setup
  hl build setup   # in CI, installs to $GITHUB_WORKSPACE/bin`,
	RunE: func(cmd *cobra.Command, args []string) error {
		binDir := setupBinDir()
		if err := os.MkdirAll(binDir, 0755); err != nil {
			return fmt.Errorf("creating bin dir: %w", err)
		}

		// Ensure binDir is on PATH for subsequent tool checks.
		os.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

		ui.Step(fmt.Sprintf("Setting up tools in %s", binDir))

		goos := runtime.GOOS
		goarch := runtime.GOARCH

		for _, t := range tools {
			if t.installed() {
				ui.StepDone(fmt.Sprintf("%s %s (already installed)", t.name, t.version))
				continue
			}

			url := t.url(goos, goarch)
			ui.Step(fmt.Sprintf("Installing %s %s", t.name, t.version))

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

			ui.StepDone(fmt.Sprintf("%s %s", t.name, t.version))
		}

		fmt.Println()
		ui.StepDone(ui.SuccessStyle.Render("All tools ready"))
		return nil
	},
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
