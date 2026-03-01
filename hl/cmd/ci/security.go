package ci

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/iamkhattar/homelab/hl/internal/lint"
	"github.com/iamkhattar/homelab/hl/internal/ui"
)

var sarifDir string
var sbomDir string

// moduleName converts a relative module path to a filename-safe identifier.
// e.g. "./hl" → "hl", "./services/butler" → "services-butler".
func moduleName(rel string) string {
	name := strings.TrimPrefix(rel, ".")
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		name = "root"
	}
	return strings.ReplaceAll(name, "/", "-")
}

var lintSarifCmd = &cobra.Command{
	Use:   "lint-sarif",
	Short: "Run linters with SARIF output (golangci-lint, ansible-lint)",
	Long: `Run golangci-lint across every Go module and ansible-lint against
the Ansible directory, producing SARIF reports for each. Non-zero exits
are tolerated so that all targets are scanned.`,
	Example: `  hl ci lint-sarif
  hl ci lint-sarif --output-dir sarif/`,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := lint.RepoRoot()
		if err != nil {
			return err
		}

		if err := os.MkdirAll(sarifDir, 0755); err != nil {
			return fmt.Errorf("creating sarif dir: %w", err)
		}

		// --- golangci-lint SARIF ---
		modules, err := lint.FindGoModules(root)
		if err != nil {
			return err
		}

		tmpDir, err := os.MkdirTemp("", "lint-sarif-*")
		if err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()

		ui.Step("Running golangci-lint (SARIF)")
		var tmpFiles []string
		for _, dir := range modules {
			rel, _ := filepath.Rel(root, dir)
			name := moduleName(rel)
			tmpFile := filepath.Join(tmpDir, fmt.Sprintf("golangci-%s.sarif", name))

			ui.KeyValue("  module", rel)
			c := exec.Command("golangci-lint", "run",
				fmt.Sprintf("--output.sarif.path=%s", tmpFile), "./...")
			c.Dir = dir
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			_ = c.Run()
			if _, err := os.Stat(tmpFile); err == nil {
				tmpFiles = append(tmpFiles, tmpFile)
			}
		}

		outFile := filepath.Join(sarifDir, "golangci-lint.sarif")
		if err := mergeSARIFFiles(tmpFiles, outFile); err != nil {
			return fmt.Errorf("merging golangci-lint SARIF: %w", err)
		}
		ui.StepDone("golangci-lint SARIF complete")

		// --- ansible-lint SARIF ---
		ansibleDir := filepath.Join(root, "ansible")
		if _, statErr := os.Stat(ansibleDir); statErr == nil {
			if _, lookErr := exec.LookPath("ansible-lint"); lookErr == nil {
				ui.Step("Running ansible-lint (SARIF)")
				ansibleSarif := filepath.Join(sarifDir, "ansible-lint.sarif")
				c := exec.Command("ansible-lint", "-f", "sarif", "--sarif-file", ansibleSarif)
				c.Dir = ansibleDir
				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				_ = c.Run() // tolerate non-zero (findings present)
				ui.StepDone("ansible-lint SARIF complete")
			} else {
				ui.KeyValue("ansible-lint", "skipped (not found)")
			}
		}

		return nil
	},
}

var gitleaksCmd = &cobra.Command{
	Use:   "gitleaks",
	Short: "Run gitleaks secret scanning with SARIF output",
	Long:  `Scan the repository for secrets using gitleaks, producing a SARIF report.`,
	Example: `  hl ci gitleaks
  hl ci gitleaks --output-dir sarif/`,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := lint.RepoRoot()
		if err != nil {
			return err
		}

		if err := os.MkdirAll(sarifDir, 0755); err != nil {
			return fmt.Errorf("creating sarif dir: %w", err)
		}

		outFile := filepath.Join(sarifDir, "gitleaks.sarif")

		ui.Step("Running gitleaks")
		c := exec.Command("gitleaks", "dir", root,
			"--report-format", "sarif",
			"--report-path", outFile)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		// Tolerate non-zero exit — gitleaks returns 1 when findings exist.
		_ = c.Run()

		ui.StepDone("gitleaks scan complete")
		return nil
	},
}

var sbomCmd = &cobra.Command{
	Use:   "sbom",
	Short: "Generate SBOMs (SPDX + CycloneDX) for all Go modules",
	Long: `Run syft against every Go module in the repo, producing
SPDX JSON and CycloneDX JSON SBOMs per module.`,
	Example: `  hl ci sbom
  hl ci sbom --output-dir sbom/`,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := lint.RepoRoot()
		if err != nil {
			return err
		}

		if err := os.MkdirAll(sbomDir, 0755); err != nil {
			return fmt.Errorf("creating sbom dir: %w", err)
		}

		modules, err := lint.FindGoModules(root)
		if err != nil {
			return err
		}

		ui.Step("Generating SBOMs (syft)")
		for _, dir := range modules {
			rel, _ := filepath.Rel(root, dir)
			name := moduleName(rel)
			spdxFile := filepath.Join(sbomDir, fmt.Sprintf("%s.spdx.json", name))
			cdxFile := filepath.Join(sbomDir, fmt.Sprintf("%s.cdx.json", name))

			ui.KeyValue("  module", rel)
			c := exec.Command("syft", "scan", fmt.Sprintf("dir:%s", dir),
				"-o", fmt.Sprintf("spdx-json=%s", spdxFile),
				"-o", fmt.Sprintf("cyclonedx-json=%s", cdxFile))
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return fmt.Errorf("syft %s: %w", rel, err)
			}
		}

		ui.StepDone("SBOM generation complete")
		return nil
	},
}

var vulnscanCmd = &cobra.Command{
	Use:   "vulnscan",
	Short: "Run vulnerability scanning (grype) against SBOMs",
	Long: `Scan SPDX SBOMs with grype, producing a single merged SARIF report.
Expects SBOMs in --sbom-dir (generated by hl ci sbom).`,
	Example: `  hl ci vulnscan
  hl ci vulnscan --sbom-dir sbom/ --output-dir sarif/`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := os.MkdirAll(sarifDir, 0755); err != nil {
			return fmt.Errorf("creating sarif dir: %w", err)
		}

		matches, err := filepath.Glob(filepath.Join(sbomDir, "*.spdx.json"))
		if err != nil {
			return err
		}
		if len(matches) == 0 {
			ui.KeyValue("vulnscan", "no SBOMs found, skipping")
			return nil
		}

		tmpDir, err := os.MkdirTemp("", "vulnscan-*")
		if err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()

		ui.Step("Running vulnerability scan (grype)")
		var tmpFiles []string
		for _, sbomFile := range matches {
			base := filepath.Base(sbomFile)
			name := strings.TrimSuffix(base, ".spdx.json")
			tmpFile := filepath.Join(tmpDir, fmt.Sprintf("grype-%s.sarif", name))

			ui.KeyValue("  scanning", name)
			c := exec.Command("grype", fmt.Sprintf("sbom:%s", sbomFile),
				"--output", "sarif", "--file", tmpFile)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			_ = c.Run()
			if _, err := os.Stat(tmpFile); err == nil {
				tmpFiles = append(tmpFiles, tmpFile)
			}
		}

		outFile := filepath.Join(sarifDir, "grype.sarif")
		if err := mergeSARIFFiles(tmpFiles, outFile); err != nil {
			return fmt.Errorf("merging SARIF: %w", err)
		}

		ui.StepDone("vulnerability scan complete")
		return nil
	},
}

// mergeSARIFFiles reads multiple SARIF files and merges all results into a
// single file with one run. This satisfies CodeQL's requirement of one run
// per category when uploading to GitHub Code Scanning.
func mergeSARIFFiles(paths []string, outFile string) error {
	// When no SARIF files were produced, write a minimal valid SARIF doc
	// so that the upload step never encounters a missing file.
	if len(paths) == 0 {
		empty := `{"$schema":"https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json","version":"2.1.0","runs":[{"tool":{"driver":{"name":"unknown","rules":[]}},"results":[]}]}`
		return os.WriteFile(outFile, []byte(empty), 0644)
	}

	type sarifDoc struct {
		Schema  string           `json:"$schema"`
		Version string           `json:"version"`
		Runs    []map[string]any `json:"runs"`
	}

	var merged *sarifDoc
	allResults := make([]any, 0) // must be non-nil so JSON encodes as [] not null

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("reading %s: %w", p, err)
		}

		var doc sarifDoc
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("parsing %s: %w", p, err)
		}

		if merged == nil {
			merged = &doc
		}

		for _, run := range doc.Runs {
			if results, ok := run["results"].([]any); ok {
				allResults = append(allResults, results...)
			}
		}
	}

	// Keep first run's tool/invocations, replace results with merged set.
	if len(merged.Runs) > 0 {
		merged.Runs[0]["results"] = allResults
		merged.Runs = merged.Runs[:1]
	}

	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outFile, out, 0644)
}

func init() {
	// SARIF output dir flag — shared by lint-sarif, gitleaks, vulnscan.
	for _, cmd := range []*cobra.Command{lintSarifCmd, gitleaksCmd, vulnscanCmd} {
		cmd.Flags().StringVar(&sarifDir, "output-dir", "sarif/", "Directory for SARIF output files")
	}

	sbomCmd.Flags().StringVar(&sbomDir, "output-dir", "sbom/", "Directory for SBOM output files")
	vulnscanCmd.Flags().StringVar(&sbomDir, "sbom-dir", "sbom/", "Directory containing SPDX SBOMs")

	Cmd.AddCommand(lintSarifCmd)
	Cmd.AddCommand(gitleaksCmd)
	Cmd.AddCommand(sbomCmd)
	Cmd.AddCommand(vulnscanCmd)
}
