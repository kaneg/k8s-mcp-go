package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	configSourceKubeconfig = "kubeconfig"
	configSourceInCluster  = "in-cluster"
	kubeconfigNotUsed      = "(not used)"
)

type kubernetesConfigInfo struct {
	Source         string
	Kubeconfig     string
	CurrentContext string
	Cluster        string
	User           string
	APIServer      string
}

type kubernetesConfigLoader struct {
	inClusterConfig func() (*rest.Config, error)
	userHomeDir     func() (string, error)
}

func newKubernetesConfigLoader() kubernetesConfigLoader {
	return kubernetesConfigLoader{
		inClusterConfig: rest.InClusterConfig,
		userHomeDir:     os.UserHomeDir,
	}
}

func (l kubernetesConfigLoader) load(explicitPath string) (*rest.Config, kubernetesConfigInfo, error) {
	if explicitPath != "" {
		return loadKubeconfig(explicitPath)
	}

	config, err := l.inClusterConfig()
	if err == nil {
		return config, kubernetesConfigInfo{
			Source:         configSourceInCluster,
			Kubeconfig:     kubeconfigNotUsed,
			CurrentContext: "(in-cluster)",
			Cluster:        "in-cluster",
			User:           "service-account",
			APIServer:      config.Host,
		}, nil
	}
	if !errors.Is(err, rest.ErrNotInCluster) {
		return nil, kubernetesConfigInfo{}, fmt.Errorf("failed to load in-cluster Kubernetes config: %w", err)
	}

	home, err := l.userHomeDir()
	if err != nil {
		return nil, kubernetesConfigInfo{}, fmt.Errorf("failed to resolve home directory for default kubeconfig: %w", err)
	}
	return loadKubeconfig(filepath.Join(home, ".kube", "config"))
}

func loadKubeconfig(path string) (*rest.Config, kubernetesConfigInfo, error) {
	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, kubernetesConfigInfo{}, fmt.Errorf("failed to load kubeconfig %q: %w", path, err)
	}

	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: path}
	rawConfig, err := loadingRules.Load()
	if err != nil {
		return nil, kubernetesConfigInfo{}, fmt.Errorf("failed to inspect kubeconfig %q: %w", path, err)
	}

	info := kubernetesConfigInfo{
		Source:         configSourceKubeconfig,
		Kubeconfig:     path,
		CurrentContext: rawConfig.CurrentContext,
		APIServer:      config.Host,
	}
	if context, ok := rawConfig.Contexts[info.CurrentContext]; ok {
		info.Cluster = context.Cluster
		info.User = context.AuthInfo
	}
	return config, info, nil
}
