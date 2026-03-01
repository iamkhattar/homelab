package build

import (
	"testing"
)

func TestCmd_HasExpectedSubcommands(t *testing.T) {
	expected := map[string]bool{
		"lint":  false,
		"fix":   false,
		"test":  false,
		"check": false,
		"docs":  false,
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

func TestCheckCmd_HasRunE(t *testing.T) {
	if checkCmd.RunE == nil {
		t.Error("checkCmd should have a RunE function")
	}
}

func TestLintCmd_HasSkipFlag(t *testing.T) {
	flag := lintCmd.Flags().Lookup("skip")
	if flag == nil {
		t.Fatal("expected --skip flag on lint command")
	}
}

func TestFixCmd_HasSkipFlag(t *testing.T) {
	flag := fixCmd.Flags().Lookup("skip")
	if flag == nil {
		t.Fatal("expected --skip flag on fix command")
	}
}

func TestCheckCmd_HasSkipFlag(t *testing.T) {
	flag := checkCmd.Flags().Lookup("skip")
	if flag == nil {
		t.Fatal("expected --skip flag on check command")
	}
}

func TestDocsCmd_HasOutputFlag(t *testing.T) {
	flag := docsCmd.Flags().Lookup("output")
	if flag == nil {
		t.Fatal("expected --output flag on docs command")
	}
	if flag.DefValue != "../docs/cli" {
		t.Errorf("expected default output '../docs/cli', got '%s'", flag.DefValue)
	}
}

func TestAllBuildSubcommands_HaveExamples(t *testing.T) {
	for _, sub := range Cmd.Commands() {
		if sub.Example == "" {
			t.Errorf("subcommand %q is missing Example field", sub.Name())
		}
	}
}

func TestAllBuildSubcommands_HaveLongDescriptions(t *testing.T) {
	for _, sub := range Cmd.Commands() {
		if sub.Long == "" {
			t.Errorf("subcommand %q is missing Long description", sub.Name())
		}
	}
}
