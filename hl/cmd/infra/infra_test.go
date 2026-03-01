package infra

import (
	"testing"
)

func TestCmd_HasExpectedSubcommands(t *testing.T) {
	expected := map[string]bool{
		"init":    false,
		"plan":    false,
		"apply":   false,
		"destroy": false,
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

func TestAllInfraSubcommands_HaveExamples(t *testing.T) {
	for _, sub := range Cmd.Commands() {
		if sub.Example == "" {
			t.Errorf("subcommand %q is missing Example field", sub.Name())
		}
	}
}

func TestAllInfraSubcommands_HaveLongDescriptions(t *testing.T) {
	for _, sub := range Cmd.Commands() {
		if sub.Long == "" {
			t.Errorf("subcommand %q is missing Long description", sub.Name())
		}
	}
}
