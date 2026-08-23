package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateDirectoryAcceptsRepositoryWorkflows(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateDirectory(root); err != nil {
		t.Fatalf("ValidateDirectory() error = %v", err)
	}
}

func TestValidateDirectoryRejectsBrokenCIContracts(t *testing.T) {
	tests := []struct {
		name        string
		transform   func(string) string
		wantInError string
	}{
		{
			name: "shallow checkout",
			transform: func(input string) string {
				return strings.Replace(input, "fetch-depth: 0", "fetch-depth: 1", 1)
			},
			wantInError: "checkout must use fetch-depth: 0",
		},
		{
			name: "missing timeout",
			transform: func(input string) string {
				return strings.Replace(input, "    timeout-minutes: 30\n", "", 1)
			},
			wantInError: "check job must define timeout-minutes",
		},
		{
			name: "missing permissions",
			transform: func(input string) string {
				return strings.Replace(input, "permissions:\n  contents: read\n", "", 1)
			},
			wantInError: "must use explicit permissions",
		},
		{
			name: "missing concurrency",
			transform: func(input string) string {
				return strings.Replace(input, "concurrency:\n  group: ci\n  cancel-in-progress: true\n", "", 1)
			},
			wantInError: "must define concurrency",
		},
		{
			name: "missing pull request trigger",
			transform: func(input string) string {
				return strings.Replace(input, "  pull_request:\n", "", 1)
			},
			wantInError: "must run for pushes and pull requests",
		},
		{
			name: "missing checkout",
			transform: func(input string) string {
				return strings.Replace(input, "actions/checkout@v7", "local/example@v1", 1)
			},
			wantInError: "check job must check out the repository",
		},
		{
			name: "workflow skips Go tests",
			transform: func(input string) string {
				return strings.Replace(input, "bin/homelabctl ci check", "bin/homelabctl ci check --skip go-test", 1)
			},
			wantInError: "check job must not skip Go tests",
		},
		{
			name: "publishes without CI guard",
			transform: func(input string) string {
				return strings.Replace(input, "          CI: \"true\"", "          CI: \"false\"", 1)
			},
			wantInError: "image publication must set CI=true",
		},
		{
			name: "old toolchain",
			transform: func(input string) string {
				return strings.Replace(input, "GO_VERSION: \"1.27.0\"", "GO_VERSION: \"1.26.0\"", 1)
			},
			wantInError: "must use Go 1.27.0",
		},
		{
			name: "old Node toolchain",
			transform: func(input string) string {
				return strings.Replace(input, "NODE_VERSION: \"24\"", "NODE_VERSION: \"22\"", 1)
			},
			wantInError: "must use Node 24",
		},
		{
			name: "publish does not depend on checks",
			transform: func(input string) string {
				return strings.Replace(input, "    needs: check\n", "", 1)
			},
			wantInError: "publish job must depend on check",
		},
		{
			name: "release does not grant write permission",
			transform: func(input string) string {
				return strings.Replace(input, "      contents: write", "      contents: read", 1)
			},
			wantInError: "release job must grant contents: write",
		},
		{
			name: "release uses a moving tag",
			transform: func(input string) string {
				return strings.Replace(input, "v0.1.${{ github.run_number }}", "latest", 1)
			},
			wantInError: "immutable semantic tag",
		},
		{
			name: "release action uses a moving tag",
			transform: func(input string) string {
				return strings.Replace(input, pinnedGoReleaserAction, "goreleaser/goreleaser-action@v7", 1)
			},
			wantInError: "reviewed commit",
		},
		{
			name: "release runs on pull requests",
			transform: func(input string) string {
				return strings.Replace(input, "    if: github.event_name == 'push' && github.ref == 'refs/heads/main'\n", "", 1)
			},
			wantInError: "only for pushes to main",
		},
		{
			name: "wrong pull request base",
			transform: func(input string) string {
				return strings.Replace(input, `origin/${{ github.base_ref }}`, "origin/main", 1)
			},
			wantInError: "must build PR changes from the base branch",
		},
		{
			name: "stale action",
			transform: func(input string) string {
				return strings.Replace(input, "actions/checkout@v7", "actions/checkout@v4", 1)
			},
			wantInError: "is stale; use v7 or newer",
		},
		{
			name: "deployment command",
			transform: func(input string) string {
				return strings.Replace(input, "          bin/homelabctl ci check", "          bin/homelabctl ci check\n          terraform apply", 1)
			},
			wantInError: `forbidden mutating command "terraform apply"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := writeWorkflows(t, test.transform(validCIWorkflow))
			err := ValidateDirectory(root)
			if err == nil || !strings.Contains(err.Error(), test.wantInError) {
				t.Fatalf("ValidateDirectory() error = %v, want error containing %q", err, test.wantInError)
			}
		})
	}
}

func TestValidateDirectoryRejectsInvalidYAML(t *testing.T) {
	root := writeWorkflows(t, "name: [")
	if err := ValidateDirectory(root); err == nil || !strings.Contains(err.Error(), "parsing") {
		t.Fatalf("ValidateDirectory() error = %v, want parsing error", err)
	}
}

func TestValidateDirectoryAcceptsImmutableActionPinsAndNeedsList(t *testing.T) {
	workflow := strings.ReplaceAll(validCIWorkflow, "actions/checkout@v7", "actions/checkout@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	workflow = strings.Replace(workflow, "needs: check", "needs: [check]", 1)
	root := writeWorkflows(t, workflow)
	if err := ValidateDirectory(root); err != nil {
		t.Fatalf("ValidateDirectory() rejected immutable action pins: %v", err)
	}
}

func TestValidateDirectoryRequiresWorkflows(t *testing.T) {
	if err := ValidateDirectory(t.TempDir()); err == nil || !strings.Contains(err.Error(), "no GitHub workflows found") {
		t.Fatalf("ValidateDirectory() error = %v, want missing-workflow error", err)
	}
}

func writeWorkflows(t *testing.T, ci string) string {
	t.Helper()
	root := t.TempDir()
	directory := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "ci.yml"), []byte(ci), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

const validCIWorkflow = `name: CI
on:
  push:
  pull_request:
permissions:
  contents: read
concurrency:
  group: ci
  cancel-in-progress: true
env:
  GO_VERSION: "1.27.0"
  NODE_VERSION: "24"
jobs:
  check:
    runs-on: ubuntu-latest
    timeout-minutes: 30
    steps:
      - uses: actions/checkout@v7
        with:
          fetch-depth: 0
      - run: |
          bin/homelabctl ci check
  publish:
    needs: check
    runs-on: ubuntu-latest
    timeout-minutes: 45
    steps:
      - uses: actions/checkout@v7
        with:
          fetch-depth: 0
      - name: Build changed images
        run: |
          bin/homelabctl ci build --changed --base "origin/${{ github.base_ref }}" --tag "${{ github.sha }}"
      - name: Publish changed images
        run: |
          bin/homelabctl ci publish --changed --base "${{ github.event.before }}" --tag latest --tag "${{ github.sha }}"
        env:
          CI: "true"
  release:
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    needs: check
    permissions:
      contents: write
    runs-on: ubuntu-latest
    timeout-minutes: 15
    steps:
      - uses: actions/checkout@v7
        with:
          fetch-depth: 0
      - uses: goreleaser/goreleaser-action@f06c13b6b1a9625abc9e6e439d9c05a8f2190e94
        with:
          version: v2.17.1
          args: release --clean
          workdir: homelabctl
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GORELEASER_CURRENT_TAG: v0.1.${{ github.run_number }}
`
