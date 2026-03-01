package exec

import (
	"os"
	"os/exec"

	"github.com/spf13/viper"
)

// KubectlArgs builds the full argument list for a kubectl command,
// prepending --kubeconfig and --context flags from config if set.
func KubectlArgs(args ...string) []string {
	kubeconfig := viper.GetString("cluster.kubeconfig")
	context := viper.GetString("cluster.context")

	fullArgs := make([]string, 0, len(args)+4)
	if kubeconfig != "" {
		fullArgs = append(fullArgs, "--kubeconfig", kubeconfig)
	}
	if context != "" {
		fullArgs = append(fullArgs, "--context", context)
	}
	fullArgs = append(fullArgs, args...)
	return fullArgs
}

// Kubectl executes a kubectl command with config-aware flags.
func Kubectl(args ...string) error {
	fullArgs := KubectlArgs(args...)
	c := exec.Command("kubectl", fullArgs...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c.Run()
}

// HelmfileArgs builds the full argument list for a helmfile command.
func HelmfileArgs(args ...string) []string {
	return args
}

// Helmfile executes a helmfile command, using helmfile.dir from config if set.
func Helmfile(args ...string) error {
	dir := viper.GetString("helmfile.dir")
	c := exec.Command("helmfile", args...)
	if dir != "" {
		c.Dir = dir
	}
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// TerraformArgs builds the full argument list for a terraform command.
func TerraformArgs(args ...string) []string {
	return args
}

// Terraform executes a terraform command, using infra.dir from config if set.
func Terraform(args ...string) error {
	dir := viper.GetString("infra.dir")
	c := exec.Command("terraform", args...)
	if dir != "" {
		c.Dir = dir
	}
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// Run executes an arbitrary command with stdout/stderr attached.
func Run(name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// RunAll executes a list of commands sequentially.
func RunAll(commands [][]string) error {
	for _, parts := range commands {
		if err := Run(parts[0], parts[1:]...); err != nil {
			return err
		}
	}
	return nil
}

// Interactive executes a command with stdin/stdout/stderr attached.
func Interactive(name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
