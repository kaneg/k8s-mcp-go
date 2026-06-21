package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/client-go/rest"
)

func TestKubernetesConfigLoader_ExplicitKubeconfigTakesPrecedence(t *testing.T) {
	path := writeTestKubeconfig(t, t.TempDir(), "https://explicit.example.com")
	inClusterCalled := false
	loader := kubernetesConfigLoader{
		inClusterConfig: func() (*rest.Config, error) {
			inClusterCalled = true
			return &rest.Config{Host: "https://in-cluster.example.com"}, nil
		},
	}

	config, info, err := loader.load(path)
	if err != nil {
		t.Fatalf("load(%q) error: %v", path, err)
	}
	if inClusterCalled {
		t.Fatal("explicit kubeconfig should not call in-cluster config")
	}
	if config.Host != "https://explicit.example.com" {
		t.Errorf("config.Host = %q, want explicit API server", config.Host)
	}
	if info.Source != configSourceKubeconfig {
		t.Errorf("info.Source = %q, want %q", info.Source, configSourceKubeconfig)
	}
	if info.Kubeconfig != path {
		t.Errorf("info.Kubeconfig = %q, want %q", info.Kubeconfig, path)
	}
}

func TestKubernetesConfigLoader_InvalidExplicitKubeconfigDoesNotFallback(t *testing.T) {
	inClusterCalled := false
	loader := kubernetesConfigLoader{
		inClusterConfig: func() (*rest.Config, error) {
			inClusterCalled = true
			return &rest.Config{Host: "https://in-cluster.example.com"}, nil
		},
	}

	_, _, err := loader.load(filepath.Join(t.TempDir(), "missing"))
	if err == nil || !strings.Contains(err.Error(), "failed to load kubeconfig") {
		t.Fatalf("load(missing) error = %v, want actionable kubeconfig error", err)
	}
	if inClusterCalled {
		t.Fatal("invalid explicit kubeconfig should not fall back to in-cluster config")
	}
}

func TestKubernetesConfigLoader_UsesInClusterConfig(t *testing.T) {
	want := &rest.Config{
		Host:            "https://10.43.0.1:443",
		BearerToken:     "initial-token",
		BearerTokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token",
	}
	loader := kubernetesConfigLoader{
		inClusterConfig: func() (*rest.Config, error) { return want, nil },
		userHomeDir: func() (string, error) {
			t.Fatal("successful in-cluster config should not inspect the home directory")
			return "", nil
		},
	}

	config, info, err := loader.load("")
	if err != nil {
		t.Fatalf("load(empty) error: %v", err)
	}
	if config != want {
		t.Fatal("loader should preserve the in-cluster rest.Config")
	}
	if config.BearerTokenFile != want.BearerTokenFile {
		t.Errorf("BearerTokenFile = %q, want %q", config.BearerTokenFile, want.BearerTokenFile)
	}
	if info.Source != configSourceInCluster || info.APIServer != want.Host {
		t.Errorf("info = %+v, want in-cluster source and API server", info)
	}
	if info.Kubeconfig != kubeconfigNotUsed || info.User != "service-account" {
		t.Errorf("info = %+v, want in-cluster identity details", info)
	}
}

func TestKubernetesConfigLoader_FallsBackToDefaultKubeconfigOutsideCluster(t *testing.T) {
	home := t.TempDir()
	path := writeTestKubeconfig(t, home, "https://default.example.com")
	loader := kubernetesConfigLoader{
		inClusterConfig: func() (*rest.Config, error) { return nil, rest.ErrNotInCluster },
		userHomeDir:     func() (string, error) { return home, nil },
	}

	config, info, err := loader.load("")
	if err != nil {
		t.Fatalf("load(empty) error: %v", err)
	}
	if config.Host != "https://default.example.com" {
		t.Errorf("config.Host = %q, want default kubeconfig API server", config.Host)
	}
	if info.Source != configSourceKubeconfig || info.Kubeconfig != path {
		t.Errorf("info = %+v, want default kubeconfig path %q", info, path)
	}
}

func TestKubernetesConfigLoader_BrokenInClusterConfigDoesNotFallback(t *testing.T) {
	homeCalled := false
	loader := kubernetesConfigLoader{
		inClusterConfig: func() (*rest.Config, error) {
			return nil, errors.New("open service account token: permission denied")
		},
		userHomeDir: func() (string, error) {
			homeCalled = true
			return t.TempDir(), nil
		},
	}

	_, _, err := loader.load("")
	if err == nil || !strings.Contains(err.Error(), "failed to load in-cluster Kubernetes config") {
		t.Fatalf("load(empty) error = %v, want actionable in-cluster error", err)
	}
	if homeCalled {
		t.Fatal("broken in-cluster config should not fall back to a local kubeconfig")
	}
}

func writeTestKubeconfig(t *testing.T, home, server string) string {
	t.Helper()
	dir := filepath.Join(home, ".kube")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config")
	content := `apiVersion: v1
kind: Config
clusters:
- name: test-cluster
  cluster:
    server: ` + server + `
    insecure-skip-tls-verify: true
users:
- name: test-user
  user:
    token: test-token
contexts:
- name: test-context
  context:
    cluster: test-cluster
    user: test-user
current-context: test-context
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
