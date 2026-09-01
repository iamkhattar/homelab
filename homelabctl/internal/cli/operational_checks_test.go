package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamkhattar/homelab/homelabctl/internal/command"
)

func TestInventoryConnectionReadsPrivateInventoryWithoutAnsible(t *testing.T) {
	repo := testRepository(t)
	writePrivateInventory(t, repo, `
k3s_cluster:
  children:
    server:
      hosts:
        titan:
          ansible_host: 192.168.1.163
          ansible_user: operator
          ansible_port: 2222
          ansible_ssh_private_key_file: /keys/titan
`)
	runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	s := &state{runner: runner, root: repo}

	got, err := s.inventoryConnection("titan")
	if err != nil {
		t.Fatalf("inventoryConnection() error = %v", err)
	}
	want := inventoryConnection{Address: "192.168.1.163", User: "operator", Port: 2222, IdentityFile: "/keys/titan"}
	if got != want {
		t.Fatalf("inventoryConnection() = %+v, want %+v", got, want)
	}
}

func TestDoctorStrictAcceptsSupportedToolchain(t *testing.T) {
	repo := doctorRepository(t)
	toolDirectory := fakeDoctorTools(t, "go version go1.27.0 linux/amd64", "v24.19.0")
	t.Setenv("PATH", toolDirectory)

	var stdout bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &stdout, &bytes.Buffer{})
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", repo, "doctor", "--strict"})
	if err := root.Execute(); err != nil {
		t.Fatalf("doctor --strict error = %v\n%s", err, stdout.String())
	}
	for _, fragment := range []string{"go version go1.27.0", "v24.19.0", "ansible/inventory/hosts.yml"} {
		if !strings.Contains(stdout.String(), fragment) {
			t.Errorf("doctor output %q does not contain %q", stdout.String(), fragment)
		}
	}
}

func TestDoctorStrictRejectsOldGoAndNode(t *testing.T) {
	repo := doctorRepository(t)
	toolDirectory := fakeDoctorTools(t, "go version go1.26.3 linux/amd64", "v22.12.0")
	t.Setenv("PATH", toolDirectory)

	var stdout bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &stdout, &bytes.Buffer{})
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", repo, "doctor", "--strict"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "2 missing requirement") {
		t.Fatalf("doctor --strict error = %v, want two unsupported tools", err)
	}
	for _, fragment := range []string{"need >=1.27", "need >=24"} {
		if !strings.Contains(stdout.String(), fragment) {
			t.Errorf("doctor output %q does not contain %q", stdout.String(), fragment)
		}
	}
}

func TestCICheckGoFormatReportsUnformattedFiles(t *testing.T) {
	repo := testRepository(t)
	path := filepath.Join(repo, "module", "unformatted.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package module\nfunc Value( )int{return 1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &stdout, &bytes.Buffer{})
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", repo, "ci", "check", "--only", "go-format"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "checks failed: go-format") {
		t.Fatalf("ci check error = %v, want formatting failure", err)
	}
	if !strings.Contains(stdout.String(), path) {
		t.Fatalf("ci check output %q does not identify %s", stdout.String(), path)
	}
}

func TestCICheckPropagatesFailingGoTests(t *testing.T) {
	repo := testRepository(t)
	module := filepath.Join(repo, "failing")
	if err := os.MkdirAll(module, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(module, "go.mod"), []byte("module example.invalid/failing\n\ngo 1.27.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(module, "failing_test.go"), []byte("package failing\nimport \"testing\"\nfunc TestFailure(t *testing.T) { t.Fatal(\"expected\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &stdout, &bytes.Buffer{})
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", repo, "ci", "check", "--only", "go-test"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "checks failed: go-test") {
		t.Fatalf("ci check error = %v, want test failure", err)
	}
	if !strings.Contains(stdout.String(), "FAIL  go-test") {
		t.Fatalf("ci check output %q does not report the failed test stage", stdout.String())
	}
}

func TestCICheckDryRunCoversAnsibleAndTerraformCommands(t *testing.T) {
	tests := []struct {
		check string
		want  []string
	}{
		{check: "ansible", want: []string{"ansible-lint --offline .", "playbooks/prepare.yml", "playbooks/recovery-export.yml", "tests/homelab-base-fstab.yml -i localhost,"}},
		{check: "terraform", want: []string{"terraform fmt -check -recursive", "terraform init -backend=false -input=false", "terraform validate", "terraform test"}},
	}
	for _, test := range tests {
		t.Run(test.check, func(t *testing.T) {
			var stderr bytes.Buffer
			runner := command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
			root := New(BuildInfo{}, runner)
			root.SetArgs([]string{"--repo-root", testRepository(t), "--dry-run", "ci", "check", "--only", test.check})
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			for _, fragment := range test.want {
				if !strings.Contains(stderr.String(), fragment) {
					t.Errorf("dry-run output %q does not contain %q", stderr.String(), fragment)
				}
			}
		})
	}
}

func doctorRepository(t *testing.T) string {
	t.Helper()
	repo := testRepository(t)
	for _, path := range []string{
		"ansible/inventory/hosts.yml",
		"ansible/playbooks/site.yml",
		"cluster/helmfile.yaml.gotmpl",
		"infra/main.tf",
		"docs/package-lock.json",
	} {
		if err := writeEmpty(filepath.Join(repo, path)); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

func fakeDoctorTools(t *testing.T, goVersion, nodeVersion string) string {
	t.Helper()
	directory := t.TempDir()
	for _, name := range []string{"npm", "ssh", "ssh-copy-id", "ansible-playbook", "ansible-lint", "kubectl", "helm", "helmfile", "terraform", "docker", "gotestsum", "gosec"} {
		writeExecutable(t, filepath.Join(directory, name), "#!/bin/sh\nexit 0\n")
	}
	writeExecutable(t, filepath.Join(directory, "go"), "#!/bin/sh\nprintf '%s\\n' '"+goVersion+"'\n")
	writeExecutable(t, filepath.Join(directory, "node"), "#!/bin/sh\nprintf '%s\\n' '"+nodeVersion+"'\n")
	return directory
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
}
