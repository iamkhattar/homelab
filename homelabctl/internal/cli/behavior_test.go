package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/iamkhattar/homelab/homelabctl/internal/command"
	"github.com/iamkhattar/homelab/homelabctl/internal/repository"
)

func TestOperationalCommandsConstructExpectedDryRuns(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "deploy diff", args: []string{"deploy", "diff"}, want: []string{"helmfile diff", "/cluster"}},
		{name: "deploy selected release", args: []string{"deploy", "apply", "cert-manager"}, want: []string{"helmfile apply --selector name=cert-manager"}},
		{name: "deploy sync", args: []string{"deploy", "sync"}, want: []string{"helmfile sync"}},
		{name: "terraform format", args: []string{"infra", "fmt"}, want: []string{"terraform fmt -check -recursive", "/infra"}},
		{name: "terraform validate", args: []string{"infra", "validate"}, want: []string{"terraform init -backend=false -input=false", "terraform validate"}},
		{name: "terraform plan", args: []string{"infra", "plan"}, want: []string{"terraform plan"}},
		{name: "cluster nodes", args: []string{"--context", "titan-admin", "cluster", "nodes"}, want: []string{"kubectl --context titan-admin get nodes -o wide --show-labels"}},
		{name: "cluster unhealthy status", args: []string{"cluster", "status"}, want: []string{"get nodes -o wide", "--field-selector=status.phase!=Running,status.phase!=Succeeded"}},
		{name: "cluster all pods", args: []string{"cluster", "status", "--all-pods"}, want: []string{"get pods --all-namespaces"}},
		{name: "inventory verbose check", args: []string{"inventory", "check", "--verbose"}, want: []string{"ansible-inventory --graph", "ansible k3s_cluster --module-name ansible.builtin.ping -vv"}},
		{name: "cluster upgrade", args: []string{"cluster", "upgrade", "--limit", "titan", "--ask-become-pass"}, want: []string{"playbooks/upgrade.yml --limit titan --ask-become-pass"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
			root := New(BuildInfo{}, runner)
			args := append([]string{"--repo-root", testRepository(t), "--dry-run"}, test.args...)
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			for _, fragment := range test.want {
				if !strings.Contains(stderr.String(), fragment) {
					t.Errorf("dry-run output %q does not contain %q", stderr.String(), fragment)
				}
			}
			if test.name == "cluster all pods" && strings.Contains(stderr.String(), "--field-selector") {
				t.Errorf("--all-pods output unexpectedly contains a field selector: %q", stderr.String())
			}
		})
	}
}

func TestBuildServicesPushesEveryExplicitTagOnlyInCI(t *testing.T) {
	t.Setenv("CI", "true")
	repo := testRepository(t)
	if err := writeEmpty(filepath.Join(repo, "services", "api", "Dockerfile")); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", repo, "--dry-run", "build", "services", "api", "--registry", "example.test/home", "--tag", "revision", "--tag", "latest", "--push"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	sha, err := repository.HeadSHA(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"docker build --build-arg VERSION=revision --build-arg COMMIT=" + sha + " --build-arg BUILD_DATE=1970-01-01T00:00:01Z --tag example.test/home/api:revision --tag example.test/home/api:latest .",
		"docker push example.test/home/api:revision",
		"docker push example.test/home/api:latest",
	} {
		if !strings.Contains(stderr.String(), fragment) {
			t.Errorf("output %q does not contain %q", stderr.String(), fragment)
		}
	}
}

func TestSelectServicesPreservesDiscoveryOrderAndSortsErrors(t *testing.T) {
	services := []repository.Service{{Name: "api"}, {Name: "butler"}, {Name: "worker"}}
	selected, err := selectServices(services, []string{"worker", "api"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := []string{selected[0].Name, selected[1].Name}, []string{"api", "worker"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("selected names = %v, want %v", got, want)
	}
	_, err = selectServices(services, []string{"zeta", "missing"})
	if err == nil || !strings.Contains(err.Error(), "missing, zeta") {
		t.Fatalf("selectServices() error = %v, want sorted unknown names", err)
	}
}

func TestControlFlagsRejectInvalidCombinationsBeforeCommands(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"ci", "check", "--only", "docs", "--skip", "terraform"}, want: "cannot be used together"},
		{args: []string{"ci", "check", "--only", "unknown"}, want: "unknown check in --only"},
		{args: []string{"ci", "check", "--skip", "unknown"}, want: "unknown check in --skip"},
		{args: []string{"setup", "unknown"}, want: "unknown setup target"},
		{args: []string{"build", "services", "api", "--changed", "--base", "main"}, want: "cannot be combined"},
		{args: []string{"build", "services", "--changed"}, want: "--base is required"},
	}

	for _, test := range tests {
		var stderr bytes.Buffer
		runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
		root := New(BuildInfo{}, runner)
		root.SetArgs(append([]string{"--repo-root", testRepository(t), "--dry-run"}, test.args...))
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("Execute(%v) error = %v, want %q", test.args, err, test.want)
		}
		if stderr.Len() != 0 {
			t.Errorf("Execute(%v) prepared an external command: %q", test.args, stderr.String())
		}
	}
}

func TestCICheckGoTestRunsEveryDiscoveredModule(t *testing.T) {
	repo := testRepository(t)
	modules := []string{
		filepath.Join(repo, "homelabctl"),
		filepath.Join(repo, "services", "api"),
	}
	for _, module := range modules {
		if err := os.MkdirAll(module, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(module, "go.mod"), []byte("module example.invalid/test\n\ngo 1.27.0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &stdout, &stderr)
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", repo, "--dry-run", "ci", "check", "--only", "go-test"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, module := range modules {
		want := "+ (" + module + ") go test ./..."
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("go-test output %q does not contain %q", stderr.String(), want)
		}
	}
	if !strings.Contains(stdout.String(), "PASS  go-test") {
		t.Fatalf("check output %q does not report go-test success", stdout.String())
	}
}

func TestCICheckReportingModeGeneratesEveryReportThroughPinnedTools(t *testing.T) {
	repo := testRepository(t)
	for _, module := range []string{"homelabctl", filepath.Join("services", "butler")} {
		directory := filepath.Join(repo, module)
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte("module example.invalid/test\n\ngo 1.27.0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var stderr bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", repo, "--dry-run", "ci", "check", "--reports", "--only", "go-test,gosec,trivy,sbom"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, fragment := range []string{
		"gotestsum --format standard-quiet --junitfile " + filepath.Join(repo, "test-results", "homelabctl.xml"),
		"gosec -track-suppressions -fmt sarif -out " + filepath.Join(repo, "sarif", "gosec-services-butler.sarif"),
		"docker run --rm --volume " + repo + ":/workspace:ro --volume " + filepath.Join(repo, "sarif") + ":/reports --volume " + filepath.Join(repo, "trivy-cache") + ":/cache --workdir /workspace " + trivyImage + " fs --cache-dir /cache --scanners vuln,misconfig,secret --severity HIGH,CRITICAL --exit-code 0 --format sarif --output /reports/trivy.sarif",
		"docker run --rm --volume " + repo + ":/workspace:ro --volume " + filepath.Join(repo, "sarif") + ":/reports --volume " + filepath.Join(repo, "trivy-cache") + ":/cache --workdir /workspace " + trivyImage + " fs --cache-dir /cache --scanners vuln,misconfig,secret --severity HIGH,CRITICAL --exit-code 1 --format table",
		"docker run --rm --volume " + repo + ":/workspace:ro --volume " + filepath.Join(repo, "sbom") + ":/reports --volume " + filepath.Join(repo, "trivy-cache") + ":/cache --workdir /workspace " + trivyImage + " fs --cache-dir /cache --format spdx-json --output /reports/homelab.spdx.json",
		"--skip-dirs ansible/.ansible --skip-dirs ansible/.venv --skip-dirs ansible/collections --skip-dirs bin",
		"--skip-dirs docs/node_modules --skip-dirs docs/.vitepress/cache --skip-dirs docs/.vitepress/dist --skip-dirs infra/.terraform --skip-dirs node_modules",
	} {
		if !strings.Contains(stderr.String(), fragment) {
			t.Errorf("reporting dry-run %q does not contain %q", stderr.String(), fragment)
		}
	}
	for _, directory := range []string{testResultsDirectory, sarifDirectory, sbomDirectory} {
		if _, err := os.Stat(filepath.Join(repo, directory)); !os.IsNotExist(err) {
			t.Errorf("dry-run created report directory %s: %v", directory, err)
		}
	}
}

func TestSetupReportsInstallsPinnedTools(t *testing.T) {
	var stderr bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", testRepository(t), "--dry-run", "setup", "reports"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, tool := range []string{
		"gotest.tools/gotestsum@" + gotestsumVersion,
		"github.com/securego/gosec/v2/cmd/gosec@" + gosecVersion,
	} {
		if !strings.Contains(stderr.String(), "go install "+tool) {
			t.Errorf("setup reports output %q does not install %s", stderr.String(), tool)
		}
	}
}

func TestSetupGoDownloadsEveryModule(t *testing.T) {
	repo := testRepository(t)
	for _, module := range []string{"homelabctl", filepath.Join("services", "butler")} {
		if err := writeEmpty(filepath.Join(repo, module, "go.mod")); err != nil {
			t.Fatal(err)
		}
	}

	var stderr bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", repo, "--dry-run", "setup", "go"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, module := range []string{"homelabctl", filepath.Join("services", "butler")} {
		fragment := "+ (" + filepath.Join(repo, module) + ") go mod download"
		if !strings.Contains(stderr.String(), fragment) {
			t.Errorf("setup go output %q does not contain %q", stderr.String(), fragment)
		}
	}
}

func TestValidationBoundaries(t *testing.T) {
	if err := validateInventoryHost("titan.home_1"); err != nil {
		t.Errorf("valid inventory host rejected: %v", err)
	}
	if err := validateReleaseName("cert-manager.v1"); err != nil {
		t.Errorf("valid release rejected: %v", err)
	}
	if err := validateTags([]string{"latest", strings.Repeat("a", 128)}); err != nil {
		t.Errorf("valid image tags rejected: %v", err)
	}
	for _, tag := range []string{"", ".leading-dot", strings.Repeat("a", 129), "bad/tag"} {
		if err := validateTags([]string{tag}); err == nil {
			t.Errorf("invalid image tag %q accepted", tag)
		}
	}
	for _, value := range []string{"", "   ", "line\nnext", "nul\x00byte"} {
		if err := validateNonBlank(value, "value"); err == nil {
			t.Errorf("invalid non-blank value %q accepted", value)
		}
	}
}

func TestRecoveryDestinationAcceptsOnlyPathsOutsideRepository(t *testing.T) {
	repo := t.TempDir()
	outside := filepath.Join(filepath.Dir(repo), "recovery-output")
	resolved, err := validateRecoveryDestination(repo, outside)
	if err != nil || resolved != outside {
		t.Fatalf("outside recovery destination = %q, %v; want %q", resolved, err, outside)
	}
	for _, destination := range []string{repo, filepath.Join(repo, "nested"), string(filepath.Separator)} {
		if _, err := validateRecoveryDestination(repo, destination); err == nil {
			t.Errorf("unsafe recovery destination %q accepted", destination)
		}
	}
}

func TestPublicKeyValidationAcceptsSupportedTypesAndRejectsBadData(t *testing.T) {
	for _, keyType := range []string{"ssh-ed25519", "ssh-rsa", "ecdsa-sha2-nistp256", "sk-ssh-ed25519@openssh.com"} {
		path := filepath.Join(t.TempDir(), "operator.pub")
		if err := os.WriteFile(path, []byte(keyType+" AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA operator\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := validatePublicKeyFile(path); err != nil {
			t.Errorf("supported key type %q rejected: %v", keyType, err)
		}
	}
	path := filepath.Join(t.TempDir(), "bad.pub")
	if err := os.WriteFile(path, []byte("ssh-ed25519 not-base64"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validatePublicKeyFile(path); err == nil {
		t.Fatal("invalid public-key data accepted")
	}
}
