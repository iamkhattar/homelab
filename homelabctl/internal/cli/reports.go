package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iamkhattar/homelab/homelabctl/internal/repository"
	"github.com/spf13/cobra"
)

const (
	testResultsDirectory = "test-results"
	sarifDirectory       = "sarif"
	sbomDirectory        = "sbom"
	trivyCacheDirectory  = "trivy-cache"
	trivyImage           = "ghcr.io/aquasecurity/trivy:0.74.0@sha256:62b1e65e8869bc4b4c6aa4fa2b21595256c7c2f6018a9d9ad61caf87187c1969"
)

func generateGoTestReports(cmd *cobra.Command, s *state, modules []string) error {
	directory, err := ensureReportDirectory(s, testResultsDirectory)
	if err != nil {
		return err
	}
	var failures []error
	for _, module := range modules {
		name, err := reportName(s.root, module)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		args := []string{
			"--format", "standard-quiet",
			"--junitfile", filepath.Join(directory, name+".xml"),
			"--jsonfile", filepath.Join(directory, name+".json"),
			"--", "./...",
		}
		if err := s.run(cmd.Context(), module, "gotestsum", args...); err != nil {
			failures = append(failures, fmt.Errorf("testing %s: %w", name, err))
		}
	}
	return errors.Join(failures...)
}

func generateGoSecurityReports(cmd *cobra.Command, s *state) error {
	modules, err := repository.GoModules(s.root)
	if err != nil {
		return err
	}
	directory, err := ensureReportDirectory(s, sarifDirectory)
	if err != nil {
		return err
	}
	var failures []error
	for _, module := range modules {
		name, err := reportName(s.root, module)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		output := filepath.Join(directory, "gosec-"+name+".sarif")
		if err := s.run(cmd.Context(), module, "gosec", "-track-suppressions", "-fmt", "sarif", "-out", output, "./..."); err != nil {
			failures = append(failures, fmt.Errorf("scanning %s with gosec: %w", name, err))
		}
	}
	return errors.Join(failures...)
}

func generateTrivySecurityReport(cmd *cobra.Command, s *state) error {
	_, err := ensureReportDirectory(s, sarifDirectory)
	if err != nil {
		return err
	}
	if _, err := ensureReportDirectory(s, trivyCacheDirectory); err != nil {
		return err
	}
	sarifArgs := trivyContainerArguments(s, sarifDirectory)
	sarifArgs = append(sarifArgs,
		"fs",
		"--cache-dir", "/cache",
		"--scanners", "vuln,misconfig,secret",
		"--severity", "HIGH,CRITICAL",
		"--exit-code", "0",
		"--format", "sarif",
		"--output", "/reports/trivy.sarif",
		"--ignorefile", "/workspace/.trivyignore.yaml",
	)
	sarifArgs = append(sarifArgs, trivySkipArguments()...)
	sarifArgs = append(sarifArgs, "/workspace")
	if err := s.run(cmd.Context(), s.root, "docker", sarifArgs...); err != nil {
		return fmt.Errorf("generating Trivy SARIF: %w", err)
	}

	// A SARIF file is intended for code-scanning ingestion and does not make
	// findings readable in the job log. Repeat the cached scan as a table and
	// use that invocation as the policy gate so a failure is actionable.
	tableArgs := trivyContainerArguments(s, sarifDirectory)
	tableArgs = append(tableArgs,
		"fs",
		"--cache-dir", "/cache",
		"--scanners", "vuln,misconfig,secret",
		"--severity", "HIGH,CRITICAL",
		"--exit-code", "1",
		"--format", "table",
		"--ignorefile", "/workspace/.trivyignore.yaml",
	)
	tableArgs = append(tableArgs, trivySkipArguments()...)
	tableArgs = append(tableArgs, "/workspace")
	return s.run(cmd.Context(), s.root, "docker", tableArgs...)
}

func generateSBOM(cmd *cobra.Command, s *state) error {
	_, err := ensureReportDirectory(s, sbomDirectory)
	if err != nil {
		return err
	}
	if _, err := ensureReportDirectory(s, trivyCacheDirectory); err != nil {
		return err
	}
	args := trivyContainerArguments(s, sbomDirectory)
	args = append(args,
		"fs",
		"--cache-dir", "/cache",
		"--format", "spdx-json",
		"--output", "/reports/homelab.spdx.json",
	)
	args = append(args, trivySkipArguments()...)
	args = append(args, "/workspace")
	return s.run(cmd.Context(), s.root, "docker", args...)
}

func trivyContainerArguments(s *state, reportDirectory string) []string {
	return []string{
		"run", "--rm",
		"--volume", s.root + ":/workspace:ro",
		"--volume", s.dir(reportDirectory) + ":/reports",
		"--volume", s.dir(trivyCacheDirectory) + ":/cache",
		"--workdir", "/workspace",
		trivyImage,
	}
}

func trivySkipArguments() []string {
	return []string{
		"--skip-dirs", ".git",
		"--skip-dirs", "ansible/.ansible",
		"--skip-dirs", "ansible/.venv",
		"--skip-dirs", "ansible/collections",
		"--skip-dirs", "bin",
		"--skip-dirs", "cluster/**/charts",
		"--skip-dirs", "docs/node_modules",
		"--skip-dirs", "docs/.vitepress/cache",
		"--skip-dirs", "docs/.vitepress/dist",
		"--skip-dirs", "docs/.vitepress/.temp",
		"--skip-files", "infra/config/.ssh/id_rsa",
		"--skip-dirs", "infra/.terraform",
		"--skip-dirs", "node_modules",
		"--skip-dirs", testResultsDirectory,
		"--skip-dirs", sarifDirectory,
		"--skip-dirs", sbomDirectory,
		"--skip-dirs", trivyCacheDirectory,
	}
}

func ensureReportDirectory(s *state, name string) (string, error) {
	path := s.dir(name)
	if s.dryRun {
		return path, nil
	}
	if err := os.MkdirAll(path, 0o750); err != nil {
		return "", fmt.Errorf("creating report directory %s: %w", path, err)
	}
	return path, nil
}

func reportName(root, module string) (string, error) {
	relative, err := filepath.Rel(root, module)
	if err != nil {
		return "", fmt.Errorf("resolving report name for %s: %w", module, err)
	}
	if relative == "." || relative == "" || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("Go module %s is not a named directory inside the repository", module)
	}
	return strings.ReplaceAll(filepath.ToSlash(relative), "/", "-"), nil
}
