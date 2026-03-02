package exec

import (
	"reflect"
	"testing"

	"github.com/spf13/viper"
)

func TestKubectlArgs_NoConfig(t *testing.T) {
	viper.Reset()
	args := KubectlArgs("get", "pods")
	expected := []string{"get", "pods"}
	if !reflect.DeepEqual(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestKubectlArgs_WithKubeconfig(t *testing.T) {
	viper.Reset()
	viper.Set("cluster.kubeconfig", "/home/user/.kube/config")
	args := KubectlArgs("get", "pods")
	expected := []string{"--kubeconfig", "/home/user/.kube/config", "get", "pods"}
	if !reflect.DeepEqual(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestKubectlArgs_WithContext(t *testing.T) {
	viper.Reset()
	viper.Set("cluster.context", "homelab")
	args := KubectlArgs("get", "nodes")
	expected := []string{"--context", "homelab", "get", "nodes"}
	if !reflect.DeepEqual(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestKubectlArgs_WithKubeconfigAndContext(t *testing.T) {
	viper.Reset()
	viper.Set("cluster.kubeconfig", "/etc/kubeconfig")
	viper.Set("cluster.context", "prod")
	args := KubectlArgs("get", "services", "-n", "default")
	expected := []string{"--kubeconfig", "/etc/kubeconfig", "--context", "prod", "get", "services", "-n", "default"}
	if !reflect.DeepEqual(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestKubectlArgs_NoArgs(t *testing.T) {
	viper.Reset()
	args := KubectlArgs()
	if len(args) != 0 {
		t.Errorf("expected empty args, got %v", args)
	}
}

func TestHelmfileArgs(t *testing.T) {
	args := HelmfileArgs("sync")
	expected := []string{"sync"}
	if !reflect.DeepEqual(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}

func TestTerraformArgs(t *testing.T) {
	args := TerraformArgs("plan")
	expected := []string{"plan"}
	if !reflect.DeepEqual(args, expected) {
		t.Errorf("expected %v, got %v", expected, args)
	}
}
