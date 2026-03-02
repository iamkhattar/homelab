package app

import (
	"testing"

	"github.com/spf13/viper"
)

func TestResolveNamespace_FlagTakesPrecedence(t *testing.T) {
	viper.Reset()
	viper.Set("cluster.namespace", "from-config")
	Namespace = "from-flag"
	defer func() { Namespace = "" }()

	ns := ResolveNamespace()
	if ns != "from-flag" {
		t.Errorf("expected 'from-flag', got '%s'", ns)
	}
}

func TestResolveNamespace_FallsBackToConfig(t *testing.T) {
	viper.Reset()
	viper.Set("cluster.namespace", "applications")
	Namespace = ""

	ns := ResolveNamespace()
	if ns != "applications" {
		t.Errorf("expected 'applications', got '%s'", ns)
	}
}

func TestResolveNamespace_DefaultsToDefault(t *testing.T) {
	viper.Reset()
	Namespace = ""

	ns := ResolveNamespace()
	if ns != "default" {
		t.Errorf("expected 'default', got '%s'", ns)
	}
}

func TestCmd_HasExpectedSubcommands(t *testing.T) {
	expected := map[string]bool{
		"list":    false,
		"status":  false,
		"logs":    false,
		"restart": false,
		"forward": false,
		"exec":    false,
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

func TestCmd_HasNamespaceFlag(t *testing.T) {
	flag := Cmd.PersistentFlags().Lookup("namespace")
	if flag == nil {
		t.Fatal("expected persistent --namespace flag")
	}
	if flag.Shorthand != "n" {
		t.Errorf("expected shorthand 'n', got '%s'", flag.Shorthand)
	}
}
