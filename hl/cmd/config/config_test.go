package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestCmd_HasExpectedSubcommands(t *testing.T) {
	expected := map[string]bool{
		"init": false,
		"show": false,
		"set":  false,
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

func TestInitCmd_CreatesConfigFile(t *testing.T) {
	// Use a temp dir as HOME so we don't touch the real config.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := initCmd.RunE(initCmd, nil)
	if err != nil {
		t.Fatalf("initCmd failed: %v", err)
	}

	configFile := filepath.Join(tmpHome, ".homelab", "config.yaml")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Fatalf("expected config file to be created at %s", configFile)
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	content := string(data)
	if len(content) == 0 {
		t.Error("config file should not be empty")
	}
}

func TestInitCmd_SkipsIfExists(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	configDir := filepath.Join(tmpHome, ".homelab")
	os.MkdirAll(configDir, 0755)
	os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("existing"), 0644)

	err := initCmd.RunE(initCmd, nil)
	if err != nil {
		t.Fatalf("initCmd failed: %v", err)
	}

	// Verify the file was not overwritten.
	data, _ := os.ReadFile(filepath.Join(configDir, "config.yaml"))
	if string(data) != "existing" {
		t.Error("initCmd should not overwrite existing config file")
	}
}

func TestSetCmd_RequiresTwoArgs(t *testing.T) {
	err := setCmd.Args(setCmd, []string{"only-one"})
	if err == nil {
		t.Error("expected error for single argument")
	}

	err = setCmd.Args(setCmd, []string{"key", "value"})
	if err != nil {
		t.Errorf("expected no error for two arguments, got: %v", err)
	}
}

func TestSetCmd_SetsViperValue(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create initial config so WriteConfig works.
	configDir := filepath.Join(tmpHome, ".homelab")
	os.MkdirAll(configDir, 0755)
	configFile := filepath.Join(configDir, "config.yaml")
	os.WriteFile(configFile, []byte("cluster:\n  namespace: default\n"), 0644)

	viper.Reset()
	viper.AddConfigPath(configDir)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.ReadInConfig()

	err := setCmd.RunE(setCmd, []string{"cluster.namespace", "production"})
	if err != nil {
		t.Fatalf("setCmd failed: %v", err)
	}

	val := viper.GetString("cluster.namespace")
	if val != "production" {
		t.Errorf("expected 'production', got '%s'", val)
	}
}
