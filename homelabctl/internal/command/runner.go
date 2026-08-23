package command

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// Runner executes external tools without involving a shell. Keeping process
// construction in one place makes dry-run output and tests deterministic.
type Runner struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	DryRun bool
}

func NewRunner(stdin io.Reader, stdout, stderr io.Writer) *Runner {
	return &Runner{Stdin: stdin, Stdout: stdout, Stderr: stderr}
}

func (r *Runner) Run(ctx context.Context, dir, name string, args ...string) error {
	return r.RunEnv(ctx, dir, nil, name, args...)
}

func (r *Runner) RunEnv(ctx context.Context, dir string, environment map[string]string, name string, args ...string) error {
	r.print(dir, name, args)
	if r.DryRun {
		return nil
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if len(environment) > 0 {
		cmd.Env = append(os.Environ(), environmentValues(environment)...)
	}
	cmd.Stdin = r.Stdin
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}

func environmentValues(environment map[string]string) []string {
	keys := make([]string, 0, len(environment))
	for key := range environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, key+"="+environment[key])
	}
	return values
}

func (r *Runner) Output(ctx context.Context, dir, name string, args ...string) (string, error) {
	return r.OutputEnv(ctx, dir, nil, name, args...)
}

func (r *Runner) OutputEnv(ctx context.Context, dir string, environment map[string]string, name string, args ...string) (string, error) {
	r.print(dir, name, args)
	if r.DryRun {
		return "", nil
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if len(environment) > 0 {
		cmd.Env = append(os.Environ(), environmentValues(environment)...)
	}
	cmd.Stdin = r.Stdin
	cmd.Stderr = r.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s failed: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *Runner) LookPath(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("required tool %q was not found on PATH", name)
	}
	return path, nil
}

func (r *Runner) print(dir, name string, args []string) {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, name)
	for _, arg := range args {
		parts = append(parts, quote(arg))
	}
	if dir == "" {
		_, _ = fmt.Fprintf(r.Stderr, "+ %s\n", strings.Join(parts, " "))
		return
	}
	_, _ = fmt.Fprintf(r.Stderr, "+ (%s) %s\n", dir, strings.Join(parts, " "))
}

func quote(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\n\"'\\$`;&|<>()[]{}*?!") {
		return value
	}
	return strconv.Quote(value)
}
