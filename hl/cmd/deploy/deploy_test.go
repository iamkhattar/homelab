package deploy

import (
	"testing"
)

func TestCmd_HasExpectedSubcommands(t *testing.T) {
	expected := map[string]bool{
		"sync":  false,
		"diff":  false,
		"apply": false,
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

func TestApplyCmd_AcceptsZeroOrOneArg(t *testing.T) {
	err := applyCmd.Args(applyCmd, []string{})
	if err != nil {
		t.Errorf("apply should accept zero args, got: %v", err)
	}

	err = applyCmd.Args(applyCmd, []string{"vault"})
	if err != nil {
		t.Errorf("apply should accept one arg, got: %v", err)
	}

	err = applyCmd.Args(applyCmd, []string{"vault", "extra"})
	if err == nil {
		t.Error("apply should reject two args")
	}
}

func TestAllDeploySubcommands_HaveExamples(t *testing.T) {
	for _, sub := range Cmd.Commands() {
		if sub.Example == "" {
			t.Errorf("subcommand %q is missing Example field", sub.Name())
		}
	}
}

func TestAllDeploySubcommands_HaveLongDescriptions(t *testing.T) {
	for _, sub := range Cmd.Commands() {
		if sub.Long == "" {
			t.Errorf("subcommand %q is missing Long description", sub.Name())
		}
	}
}
