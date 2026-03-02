package cluster

import (
	"testing"
)

func TestCmd_HasExpectedSubcommands(t *testing.T) {
	expected := map[string]bool{
		"status":     false,
		"nodes":      false,
		"kubeconfig": false,
		"ssh":        false,
		"bootstrap":  false,
	}

	for _, sub := range Cmd.Commands() {
		if _, ok := expected[sub.Name()]; ok {
			expected[sub.Name()] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("expected subcommand %q not found", name)
		}
	}
}

func TestSshCmd_RequiresExactlyOneArg(t *testing.T) {
	err := sshCmd.Args(sshCmd, []string{})
	if err == nil {
		t.Error("ssh should require exactly one argument")
	}

	err = sshCmd.Args(sshCmd, []string{"node-0"})
	if err != nil {
		t.Errorf("ssh should accept one argument, got: %v", err)
	}

	err = sshCmd.Args(sshCmd, []string{"a", "b"})
	if err == nil {
		t.Error("ssh should reject two arguments")
	}
}

func TestBootstrapCmd_RequiresExactlyOneArg(t *testing.T) {
	err := bootstrapCmd.Args(bootstrapCmd, []string{})
	if err == nil {
		t.Error("bootstrap should require exactly one argument")
	}

	err = bootstrapCmd.Args(bootstrapCmd, []string{"inventory-server.yml"})
	if err != nil {
		t.Errorf("bootstrap should accept one argument, got: %v", err)
	}
}

func TestAllClusterSubcommands_HaveExamples(t *testing.T) {
	for _, sub := range Cmd.Commands() {
		if sub.Example == "" {
			t.Errorf("subcommand %q is missing Example field", sub.Name())
		}
	}
}

func TestAllClusterSubcommands_HaveLongDescriptions(t *testing.T) {
	for _, sub := range Cmd.Commands() {
		if sub.Long == "" {
			t.Errorf("subcommand %q is missing Long description", sub.Name())
		}
	}
}
