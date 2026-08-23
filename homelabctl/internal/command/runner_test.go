package command

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDryRunPrintsCommandWithoutExecuting(t *testing.T) {
	var stderr bytes.Buffer
	runner := NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
	runner.DryRun = true

	if err := runner.Run(context.Background(), "/repo/ansible", "ansible-playbook", "playbooks/site.yml", "--limit", "titan"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := stderr.String()
	want := "+ (/repo/ansible) ansible-playbook playbooks/site.yml --limit titan\n"
	if got != want {
		t.Fatalf("dry-run output = %q, want %q", got, want)
	}
}

func TestQuoteProtectsWhitespace(t *testing.T) {
	if got, want := quote("origin/main branch"), `"origin/main branch"`; got != want {
		t.Fatalf("quote() = %q, want %q", got, want)
	}
}

func TestEnvironmentValuesAreStable(t *testing.T) {
	got := environmentValues(map[string]string{"B": "two", "A": "one"})
	if strings.Join(got, ",") != "A=one,B=two" {
		t.Fatalf("environmentValues() = %v", got)
	}
}

func TestOutputEnvUsesWorkingDirectoryAndEnvironment(t *testing.T) {
	directory := t.TempDir()
	var stderr bytes.Buffer
	runner := NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)

	out, err := runner.OutputEnv(context.Background(), directory, map[string]string{
		"GO_WANT_RUNNER_HELPER": "1",
		"RUNNER_TEST_VALUE":     "from-environment",
	}, os.Args[0], "-test.run=TestRunnerHelperProcess", "--", "argument with spaces")
	if err != nil {
		t.Fatalf("OutputEnv() error = %v", err)
	}
	resolvedDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{resolvedDirectory, "from-environment", "argument with spaces"}, "|")
	if out != want {
		t.Fatalf("OutputEnv() = %q, want %q", out, want)
	}
	if !strings.Contains(stderr.String(), filepath.Base(os.Args[0])) || !strings.Contains(stderr.String(), `"argument with spaces"`) {
		t.Fatalf("OutputEnv() command log = %q", stderr.String())
	}
}

func TestOutputEnvWrapsProcessFailure(t *testing.T) {
	runner := NewRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	_, err := runner.OutputEnv(context.Background(), t.TempDir(), map[string]string{
		"GO_WANT_RUNNER_HELPER": "1",
		"RUNNER_TEST_EXIT":      "1",
	}, os.Args[0], "-test.run=TestRunnerHelperProcess")
	if err == nil || !strings.Contains(err.Error(), filepath.Base(os.Args[0])+" failed") {
		t.Fatalf("OutputEnv() error = %v, want wrapped executable failure", err)
	}
}

func TestRunEnvForwardsStdinStdoutAndEnvironment(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(strings.NewReader("operator-input"), &stdout, &stderr)
	err := runner.RunEnv(context.Background(), t.TempDir(), map[string]string{
		"GO_WANT_RUNNER_HELPER": "1",
		"RUNNER_TEST_STDIN":     "1",
		"RUNNER_TEST_VALUE":     "environment-value",
	}, os.Args[0], "-test.run=TestRunnerHelperProcess")
	if err != nil {
		t.Fatalf("RunEnv() error = %v", err)
	}
	if got, want := strings.TrimSpace(stdout.String()), "environment-value|operator-input"; got != want {
		t.Fatalf("RunEnv() output = %q, want %q", got, want)
	}
}

func TestRunEnvWrapsProcessFailure(t *testing.T) {
	runner := NewRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	err := runner.RunEnv(context.Background(), t.TempDir(), map[string]string{
		"GO_WANT_RUNNER_HELPER": "1",
		"RUNNER_TEST_EXIT":      "1",
	}, os.Args[0], "-test.run=TestRunnerHelperProcess")
	if err == nil || !strings.Contains(err.Error(), filepath.Base(os.Args[0])+" failed") {
		t.Fatalf("RunEnv() error = %v, want wrapped executable failure", err)
	}
}

func TestOutputDryRunDoesNotExecute(t *testing.T) {
	var stderr bytes.Buffer
	runner := NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr)
	runner.DryRun = true
	out, err := runner.Output(context.Background(), "", "tool-that-does-not-exist", "argument")
	if err != nil || out != "" {
		t.Fatalf("Output() = %q, %v; want empty dry-run result", out, err)
	}
	if got, want := stderr.String(), "+ tool-that-does-not-exist argument\n"; got != want {
		t.Fatalf("Output() log = %q, want %q", got, want)
	}
}

func TestLookPathReportsMissingTool(t *testing.T) {
	runner := NewRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	_, err := runner.LookPath("homelabctl-tool-that-does-not-exist")
	if err == nil || !strings.Contains(err.Error(), "was not found on PATH") {
		t.Fatalf("LookPath() error = %v, want missing-tool message", err)
	}
}

func TestLookPathFindsExistingTool(t *testing.T) {
	runner := NewRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	path, err := runner.LookPath(filepath.Base(os.Args[0]))
	if err == nil && path == "" {
		t.Fatal("LookPath() returned an empty path without an error")
	}
	// The test binary is not guaranteed to be on PATH. Verify a known absolute
	// executable as the portable success case instead.
	path, err = runner.LookPath(os.Args[0])
	if err != nil || path == "" {
		t.Fatalf("LookPath(%q) = %q, %v", os.Args[0], path, err)
	}
}

func TestQuoteProtectsShellMetacharactersAndEmptyValues(t *testing.T) {
	for input, want := range map[string]string{
		"":           `""`,
		"plain":      "plain",
		"$HOME":      `"$HOME"`,
		"a;b":        `"a;b"`,
		"line\nnext": `"line\nnext"`,
	} {
		if got := quote(input); got != want {
			t.Errorf("quote(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRunnerHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_RUNNER_HELPER") != "1" {
		return
	}
	if os.Getenv("RUNNER_TEST_EXIT") == "1" {
		os.Exit(7)
	}
	if os.Getenv("RUNNER_TEST_STDIN") == "1" {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			os.Exit(9)
		}
		_, _ = fmt.Fprintf(os.Stdout, "%s|%s\n", os.Getenv("RUNNER_TEST_VALUE"), input)
		os.Exit(0)
	}
	directory, err := os.Getwd()
	if err != nil {
		os.Exit(8)
	}
	argument := ""
	for index, candidate := range os.Args {
		if candidate == "--" && index+1 < len(os.Args) {
			argument = os.Args[index+1]
		}
	}
	_, _ = fmt.Fprintf(os.Stdout, "%s|%s|%s\n", directory, os.Getenv("RUNNER_TEST_VALUE"), argument)
	os.Exit(0)
}
