package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var version = "dev"

// --- Permission modes ---

const (
	ModeReadOnly  = "readonly"
	ModeReadWrite = "readwrite"
	ModeDangerous = "dangerous"
)

// --- Input types ---

type NamespaceInput struct {
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace (default: default)"`
}

type PodInput struct {
	Name      string `json:"name" jsonschema:"Pod name"`
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace (default: default)"`
}

type LogsInput struct {
	Name      string `json:"name" jsonschema:"Pod name"`
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace (default: default)"`
	Container string `json:"container,omitempty" jsonschema:"Container name (optional, for multi-container pods)"`
	TailLines int    `json:"tail_lines,omitempty" jsonschema:"Number of lines from the end (default: 100)"`
	Previous  bool   `json:"previous,omitempty" jsonschema:"Get logs from previous container instance (default: false)"`
}

type ServiceInput struct {
	Name      string `json:"name" jsonschema:"Service name"`
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace (default: default)"`
}

type DeploymentInput struct {
	Name      string `json:"name" jsonschema:"Deployment name"`
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace (default: default)"`
}

type ScaleInput struct {
	Name      string `json:"name" jsonschema:"Deployment name"`
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace (default: default)"`
	Replicas  int32  `json:"replicas" jsonschema:"Desired number of replicas"`
}

type SetImageInput struct {
	Name       string `json:"name" jsonschema:"Deployment name"`
	Namespace  string `json:"namespace" jsonschema:"Kubernetes namespace (default: default)"`
	Container  string `json:"container" jsonschema:"Container name"`
	Image      string `json:"image" jsonschema:"New container image (e.g. nginx:1.25)"`
}

type DeletePodInput struct {
	Name      string `json:"name" jsonschema:"Pod name"`
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace (default: default)"`
	GracePeriod *int64 `json:"grace_period,omitempty" jsonschema:"Grace period in seconds (default: 30)"`
}

type ExecInput struct {
	Name      string   `json:"name" jsonschema:"Pod name"`
	Namespace string   `json:"namespace" jsonschema:"Kubernetes namespace (default: default)"`
	Container string   `json:"container,omitempty" jsonschema:"Container name (optional)"`
	Command   []string `json:"command" jsonschema:"Command to execute (e.g. [\"sh\", \"-c\", \"ls -la\"])"`
}

type ApplyYAMLInput struct {
	YAML string `json:"yaml" jsonschema:"Kubernetes YAML manifest to apply"`
}

type ConfigMapInput struct {
	Name      string `json:"name" jsonschema:"ConfigMap name"`
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace (default: default)"`
}

type SecretInput struct {
	Name      string `json:"name" jsonschema:"Secret name"`
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace (default: default)"`
	ShowValues bool  `json:"show_values,omitempty" jsonschema:"Show secret values (requires dangerous mode, default: false)"`
}

type IngressInput struct {
	Name      string `json:"name" jsonschema:"Ingress name"`
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace (default: default)"`
}

type EventsInput struct {
	Namespace string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace (default: all namespaces)"`
	Kind      string `json:"kind,omitempty" jsonschema:"Filter by resource kind (e.g. Pod, Deployment)"`
	Name      string `json:"name,omitempty" jsonschema:"Filter by resource name"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Max events to return (default: 50)"`
}

type RolloutInput struct {
	Name      string `json:"name" jsonschema:"Deployment name"`
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace (default: default)"`
}

type StatefulSetInput struct {
	Name      string `json:"name" jsonschema:"StatefulSet name"`
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace (default: default)"`
}

type PatchInput struct {
	Name      string `json:"name" jsonschema:"Resource name"`
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace (default: default)"`
	Patch     string `json:"patch" jsonschema:"JSON patch string"`
}

type EmptyInput struct{}

// --- Main ---

func main() {
	// CLI flags
	mode := flag.String("mode", ModeReadOnly, "Permission mode: readonly (default), readwrite, dangerous")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("k8s-mcp-go %s\n", version)
		os.Exit(0)
	}

	// Validate mode
	switch *mode {
	case ModeReadOnly, ModeReadWrite, ModeDangerous:
	default:
		fmt.Fprintf(os.Stderr, "Invalid mode: %s. Use: readonly, readwrite, dangerous\n", *mode)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		logger.Error("failed to load kubeconfig", "path", kubeconfig, "error", err)
		os.Exit(1)
	}

	k8s, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Error("failed to create kubernetes client", "error", err)
		os.Exit(1)
	}

	logger.Info("k8s-mcp-go starting",
		"version", version,
		"mode", *mode,
		"kubeconfig", kubeconfig,
	)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "k8s-mcp-go",
		Version: version,
	}, nil)

	// Mode check helpers
	requireReadWrite := func(tool string) bool {
		if *mode == ModeReadOnly {
			return false
		}
		return true
	}
	requireDangerous := func(tool string) bool {
		if *mode != ModeDangerous {
			return false
		}
		return true
	}

	// ==================== READ-ONLY TOOLS ====================

	// list_pods
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_pods",
		Description: "[READONLY] List all pods in a namespace with status, readiness, restarts, and age",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input NamespaceInput) (*mcp.CallToolResult, any, error) {
		ns := nsOrDefault(input.Namespace)
		pods, err := k8s.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return errResult("failed to list pods: %v", err), nil, nil
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("Pods in namespace '%s' (%d total):\n", ns, len(pods.Items)))
		lines = append(lines, "NAME | STATUS | READY | RESTARTS | AGE | NODE")
		lines = append(lines, "-----|--------|-------|----------|-----|-----")
		for _, p := range pods.Items {
			ready := 0
			total := len(p.Spec.Containers)
			restarts := int32(0)
			for _, cs := range p.Status.ContainerStatuses {
				if cs.Ready {
					ready++
				}
				restarts += cs.RestartCount
			}
			lines = append(lines, fmt.Sprintf("%s | %s | %d/%d | %d | %s | %s",
				p.Name, string(p.Status.Phase), ready, total, restarts, ageStr(p.CreationTimestamp), p.Spec.NodeName))
		}
		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// get_pod
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_pod",
		Description: "[READONLY] Get detailed information about a specific pod including containers, status, and conditions",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input PodInput) (*mcp.CallToolResult, any, error) {
		if input.Name == "" {
			return errResult("name is required"), nil, nil
		}
		ns := nsOrDefault(input.Namespace)
		pod, err := k8s.CoreV1().Pods(ns).Get(ctx, input.Name, metav1.GetOptions{})
		if err != nil {
			return errResult("failed to get pod: %v", err), nil, nil
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("Pod: %s/%s", ns, pod.Name))
		lines = append(lines, fmt.Sprintf("Status: %s", string(pod.Status.Phase)))
		lines = append(lines, fmt.Sprintf("Node: %s", pod.Spec.NodeName))
		lines = append(lines, fmt.Sprintf("IP: %s", pod.Status.PodIP))
		lines = append(lines, fmt.Sprintf("Created: %s (%s ago)", pod.CreationTimestamp.Format(time.RFC3339), ageStr(pod.CreationTimestamp)))

		if len(pod.Labels) > 0 {
			lines = append(lines, "\nLabels:")
			for k, v := range pod.Labels {
				lines = append(lines, fmt.Sprintf("  %s: %s", k, v))
			}
		}

		lines = append(lines, "\nContainers:")
		for _, c := range pod.Spec.Containers {
			lines = append(lines, fmt.Sprintf("  %s: %s", c.Name, c.Image))
		}

		if len(pod.Status.ContainerStatuses) > 0 {
			lines = append(lines, "\nContainer Status:")
			for _, cs := range pod.Status.ContainerStatuses {
				state := "unknown"
				if cs.State.Running != nil {
					state = "running"
				} else if cs.State.Waiting != nil {
					state = fmt.Sprintf("waiting (%s)", cs.State.Waiting.Reason)
				} else if cs.State.Terminated != nil {
					state = fmt.Sprintf("terminated (%s)", cs.State.Terminated.Reason)
				}
				lines = append(lines, fmt.Sprintf("  %s: %s | restarts: %d | ready: %v",
					cs.Name, state, cs.RestartCount, cs.Ready))
			}
		}

		if len(pod.Status.Conditions) > 0 {
			lines = append(lines, "\nConditions:")
			for _, cond := range pod.Status.Conditions {
				if cond.Status != "True" {
					lines = append(lines, fmt.Sprintf("  %s: %s - %s", cond.Type, cond.Status, cond.Message))
				}
			}
		}
		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// get_pod_logs
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_pod_logs",
		Description: "[READONLY] Get logs from a pod. Supports container selection, tail lines, and previous instance",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input LogsInput) (*mcp.CallToolResult, any, error) {
		if input.Name == "" {
			return errResult("name is required"), nil, nil
		}
		ns := nsOrDefault(input.Namespace)
		tail := int64(100)
		if input.TailLines > 0 {
			tail = int64(input.TailLines)
		}

		opts := &corev1.PodLogOptions{TailLines: &tail, Previous: input.Previous}
		if input.Container != "" {
			opts.Container = input.Container
		}

		req := k8s.CoreV1().Pods(ns).GetLogs(input.Name, opts)
		stream, err := req.Stream(ctx)
		if err != nil {
			return errResult("failed to get logs: %v", err), nil, nil
		}
		defer stream.Close()

		buf := make([]byte, 64*1024)
		var sb strings.Builder
		for {
			n, readErr := stream.Read(buf)
			if n > 0 {
				sb.Write(buf[:n])
			}
			if readErr != nil {
				break
			}
		}

		previous := ""
		if input.Previous {
			previous = " (previous instance)"
		}
		header := fmt.Sprintf("Logs for pod %s/%s%s (last %d lines):\n\n", ns, input.Name, previous, tail)
		return textResult(header + sb.String()), nil, nil
	})

	// list_services
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_services",
		Description: "[READONLY] List all services in a namespace with type, cluster IP, external IP, and ports",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input NamespaceInput) (*mcp.CallToolResult, any, error) {
		ns := nsOrDefault(input.Namespace)
		svcs, err := k8s.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return errResult("failed to list services: %v", err), nil, nil
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("Services in namespace '%s' (%d total):\n", ns, len(svcs.Items)))
		lines = append(lines, "NAME | TYPE | CLUSTER-IP | EXTERNAL-IP | PORTS | AGE")
		lines = append(lines, "-----|------|-----------|-------------|-------|----")
		for _, s := range svcs.Items {
			extIP := "<none>"
			if len(s.Spec.ExternalIPs) > 0 {
				extIP = strings.Join(s.Spec.ExternalIPs, ",")
			} else if s.Spec.Type == "LoadBalancer" && len(s.Status.LoadBalancer.Ingress) > 0 {
				var ips []string
				for _, ing := range s.Status.LoadBalancer.Ingress {
					if ing.IP != "" {
						ips = append(ips, ing.IP)
					} else if ing.Hostname != "" {
						ips = append(ips, ing.Hostname)
					}
				}
				if len(ips) > 0 {
					extIP = strings.Join(ips, ",")
				} else {
					extIP = "<pending>"
				}
			}

			var ports []string
			for _, p := range s.Spec.Ports {
				ports = append(ports, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
			}

			clusterIP := s.Spec.ClusterIP
			if clusterIP == "None" {
				clusterIP = "Headless"
			}

			lines = append(lines, fmt.Sprintf("%s | %s | %s | %s | %s | %s",
				s.Name, string(s.Spec.Type), clusterIP, extIP, strings.Join(ports, ","), ageStr(s.CreationTimestamp)))
		}
		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// get_service
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_service",
		Description: "[READONLY] Get detailed information about a service including ports, selectors, and endpoints",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ServiceInput) (*mcp.CallToolResult, any, error) {
		if input.Name == "" {
			return errResult("name is required"), nil, nil
		}
		ns := nsOrDefault(input.Namespace)
		svc, err := k8s.CoreV1().Services(ns).Get(ctx, input.Name, metav1.GetOptions{})
		if err != nil {
			return errResult("failed to get service: %v", err), nil, nil
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("Service: %s/%s", ns, svc.Name))
		lines = append(lines, fmt.Sprintf("Type: %s", string(svc.Spec.Type)))
		lines = append(lines, fmt.Sprintf("ClusterIP: %s", svc.Spec.ClusterIP))

		if len(svc.Spec.Ports) > 0 {
			lines = append(lines, "\nPorts:")
			for _, p := range svc.Spec.Ports {
				target := p.TargetPort.String()
				lines = append(lines, fmt.Sprintf("  %s: %d -> %s/%s", p.Name, p.Port, target, p.Protocol))
			}
		}

		if len(svc.Spec.Selector) > 0 {
			lines = append(lines, "\nSelector:")
			for k, v := range svc.Spec.Selector {
				lines = append(lines, fmt.Sprintf("  %s: %s", k, v))
			}

			endpoints, epErr := k8s.CoreV1().Endpoints(ns).Get(ctx, input.Name, metav1.GetOptions{})
			if epErr == nil && len(endpoints.Subsets) > 0 {
				lines = append(lines, "\nEndpoints:")
				for _, subset := range endpoints.Subsets {
					var addrs []string
					for _, addr := range subset.Addresses {
						addrs = append(addrs, addr.IP)
					}
					for _, port := range subset.Ports {
						lines = append(lines, fmt.Sprintf("  %s: %d/%s -> %v",
							port.Name, port.Port, port.Protocol, addrs))
					}
				}
			}
		}
		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// list_deployments
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_deployments",
		Description: "[READONLY] List all deployments in a namespace with replica status",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input NamespaceInput) (*mcp.CallToolResult, any, error) {
		ns := nsOrDefault(input.Namespace)
		deploys, err := k8s.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return errResult("failed to list deployments: %v", err), nil, nil
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("Deployments in namespace '%s' (%d total):\n", ns, len(deploys.Items)))
		lines = append(lines, "NAME | READY | UP-TO-DATE | AVAILABLE | AGE")
		lines = append(lines, "-----|-------|-----------|-----------|----")
		for _, d := range deploys.Items {
			var desired int32
			if d.Spec.Replicas != nil {
				desired = *d.Spec.Replicas
			}
			lines = append(lines, fmt.Sprintf("%s | %d/%d | %d | %d | %s",
				d.Name, d.Status.ReadyReplicas, desired, d.Status.UpdatedReplicas, d.Status.AvailableReplicas, ageStr(d.CreationTimestamp)))
		}
		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// get_deployment
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_deployment",
		Description: "[READONLY] Get detailed deployment info including replica status, strategy, containers, and conditions",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input DeploymentInput) (*mcp.CallToolResult, any, error) {
		if input.Name == "" {
			return errResult("name is required"), nil, nil
		}
		ns := nsOrDefault(input.Namespace)
		d, err := k8s.AppsV1().Deployments(ns).Get(ctx, input.Name, metav1.GetOptions{})
		if err != nil {
			return errResult("failed to get deployment: %v", err), nil, nil
		}

		var lines []string
		var desired int32
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}
		lines = append(lines, fmt.Sprintf("Deployment: %s/%s", ns, d.Name))
		lines = append(lines, fmt.Sprintf("Replicas: %d desired | %d updated | %d ready | %d available",
			desired, d.Status.UpdatedReplicas, d.Status.ReadyReplicas, d.Status.AvailableReplicas))
		lines = append(lines, fmt.Sprintf("Strategy: %s", string(d.Spec.Strategy.Type)))

		if len(d.Spec.Template.Spec.Containers) > 0 {
			lines = append(lines, "\nContainers:")
			for _, c := range d.Spec.Template.Spec.Containers {
				lines = append(lines, fmt.Sprintf("  %s: %s", c.Name, c.Image))
			}
		}

		if len(d.Status.Conditions) > 0 {
			lines = append(lines, "\nConditions:")
			for _, cond := range d.Status.Conditions {
				lines = append(lines, fmt.Sprintf("  %s: %s - %s", cond.Type, cond.Status, cond.Message))
			}
		}
		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// list_statefulsets
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_statefulsets",
		Description: "[READONLY] List all StatefulSets in a namespace with replica status",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input NamespaceInput) (*mcp.CallToolResult, any, error) {
		ns := nsOrDefault(input.Namespace)
		stss, err := k8s.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return errResult("failed to list statefulsets: %v", err), nil, nil
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("StatefulSets in namespace '%s' (%d total):\n", ns, len(stss.Items)))
		lines = append(lines, "NAME | READY | AGE")
		lines = append(lines, "-----|-------|----")
		for _, s := range stss.Items {
			var desired int32
			if s.Spec.Replicas != nil {
				desired = *s.Spec.Replicas
			}
			lines = append(lines, fmt.Sprintf("%s | %d/%d | %s",
				s.Name, s.Status.ReadyReplicas, desired, ageStr(s.CreationTimestamp)))
		}
		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// get_statefulset
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_statefulset",
		Description: "[READONLY] Get detailed StatefulSet info including replica status, update strategy, and containers",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input StatefulSetInput) (*mcp.CallToolResult, any, error) {
		if input.Name == "" {
			return errResult("name is required"), nil, nil
		}
		ns := nsOrDefault(input.Namespace)
		s, err := k8s.AppsV1().StatefulSets(ns).Get(ctx, input.Name, metav1.GetOptions{})
		if err != nil {
			return errResult("failed to get statefulset: %v", err), nil, nil
		}

		var lines []string
		var desired int32
		if s.Spec.Replicas != nil {
			desired = *s.Spec.Replicas
		}
		lines = append(lines, fmt.Sprintf("StatefulSet: %s/%s", ns, s.Name))
		lines = append(lines, fmt.Sprintf("Replicas: %d desired | %d ready | %d updated | %d available",
			desired, s.Status.ReadyReplicas, s.Status.UpdatedReplicas, s.Status.AvailableReplicas))
		lines = append(lines, fmt.Sprintf("ServiceName: %s", s.Spec.ServiceName))
		updateStrategy := "RollingUpdate"
		if s.Spec.UpdateStrategy.Type != "" {
			updateStrategy = string(s.Spec.UpdateStrategy.Type)
		}
		lines = append(lines, fmt.Sprintf("UpdateStrategy: %s", updateStrategy))

		if len(s.Spec.Template.Spec.Containers) > 0 {
			lines = append(lines, "\nContainers:")
			for _, c := range s.Spec.Template.Spec.Containers {
				lines = append(lines, fmt.Sprintf("  %s: %s", c.Name, c.Image))
			}
		}

		if len(s.Status.Conditions) > 0 {
			lines = append(lines, "\nConditions:")
			for _, cond := range s.Status.Conditions {
				lines = append(lines, fmt.Sprintf("  %s: %s - %s", cond.Type, cond.Status, cond.Message))
			}
		}
		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// list_namespaces
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_namespaces",
		Description: "[READONLY] List all Kubernetes namespaces with status and age",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input EmptyInput) (*mcp.CallToolResult, any, error) {
		nss, err := k8s.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			return errResult("failed to list namespaces: %v", err), nil, nil
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("Namespaces (%d total):\n", len(nss.Items)))
		lines = append(lines, "NAME | STATUS | AGE")
		lines = append(lines, "-----|--------|----")
		for _, ns := range nss.Items {
			lines = append(lines, fmt.Sprintf("%s | %s | %s",
				ns.Name, string(ns.Status.Phase), ageStr(ns.CreationTimestamp)))
		}
		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// list_nodes
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_nodes",
		Description: "[READONLY] List all cluster nodes with status, roles, version, and OS info",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input EmptyInput) (*mcp.CallToolResult, any, error) {
		nodes, err := k8s.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return errResult("failed to list nodes: %v", err), nil, nil
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("Nodes (%d total):\n", len(nodes.Items)))
		lines = append(lines, "NAME | STATUS | ROLES | VERSION | OS | AGE")
		lines = append(lines, "-----|--------|-------|---------|-----|----")
		for _, n := range nodes.Items {
			status := "NotReady"
			for _, c := range n.Status.Conditions {
				if c.Type == "Ready" && c.Status == "True" {
					status = "Ready"
				}
			}

			roles := "<none>"
			var roleList []string
			for k := range n.Labels {
				if strings.HasPrefix(k, "node-role.kubernetes.io/") {
					roleList = append(roleList, strings.TrimPrefix(k, "node-role.kubernetes.io/"))
				}
			}
			if len(roleList) > 0 {
				roles = strings.Join(roleList, ",")
			}

			lines = append(lines, fmt.Sprintf("%s | %s | %s | %s | %s (%s) | %s",
				n.Name, status, roles, n.Status.NodeInfo.KubeletVersion,
				n.Status.NodeInfo.OSImage, n.Status.NodeInfo.KernelVersion, ageStr(n.CreationTimestamp)))
		}
		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// cluster_overview
	mcp.AddTool(server, &mcp.Tool{
		Name:        "cluster_overview",
		Description: "[READONLY] Get a high-level cluster overview: node health, pod counts, resource summary, and problem pods",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input EmptyInput) (*mcp.CallToolResult, any, error) {
		var lines []string
		lines = append(lines, "=== Cluster Overview ===\n")

		nodes, err := k8s.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return errResult("failed to list nodes: %v", err), nil, nil
		}

		readyNodes := 0
		var totalCPU, totalMem int64
		for _, n := range nodes.Items {
			for _, c := range n.Status.Conditions {
				if c.Type == "Ready" && c.Status == "True" {
					readyNodes++
				}
			}
			totalCPU += n.Status.Capacity.Cpu().MilliValue()
			totalMem += n.Status.Capacity.Memory().Value()
		}
		lines = append(lines, fmt.Sprintf("Nodes: %d/%d ready", readyNodes, len(nodes.Items)))
		lines = append(lines, fmt.Sprintf("Total CPU: %d cores", totalCPU/1000))
		lines = append(lines, fmt.Sprintf("Total Memory: %dGi", totalMem/(1024*1024*1024)))

		nss, _ := k8s.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		lines = append(lines, fmt.Sprintf("\nNamespaces: %d", len(nss.Items)))

		var totalPods, runningPods, failedPods int
		var totalRestarts int64
		var problemPods []string

		for _, ns := range nss.Items {
			pods, _ := k8s.CoreV1().Pods(ns.Name).List(ctx, metav1.ListOptions{})
			for _, p := range pods.Items {
				totalPods++
				if p.Status.Phase == "Running" {
					runningPods++
				} else if p.Status.Phase == "Failed" {
					failedPods++
				}

				for _, cs := range p.Status.ContainerStatuses {
					totalRestarts += int64(cs.RestartCount)
					if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
						problemPods = append(problemPods, fmt.Sprintf("  - %s/%s: CrashLoopBackOff", ns.Name, p.Name))
					}
				}
			}
		}

		lines = append(lines, fmt.Sprintf("\nPods: %d total | %d running | %d failed", totalPods, runningPods, failedPods))
		lines = append(lines, fmt.Sprintf("Total restarts: %d", totalRestarts))

		if len(problemPods) > 0 {
			lines = append(lines, fmt.Sprintf("\nProblem Pods (%d):", len(problemPods)))
			lines = append(lines, problemPods...)
		} else {
			lines = append(lines, "\nNo problem pods detected.")
		}

		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// get_events (NEW)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_events",
		Description: "[READONLY] Get Kubernetes events, optionally filtered by namespace, kind, and name",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input EventsInput) (*mcp.CallToolResult, any, error) {
		limit := 50
		if input.Limit > 0 {
			limit = input.Limit
		}

		var opts metav1.ListOptions
		if input.Kind != "" && input.Name != "" {
			opts.FieldSelector = fmt.Sprintf("involvedObject.kind=%s,involvedObject.name=%s", input.Kind, input.Name)
		} else if input.Kind != "" {
			opts.FieldSelector = fmt.Sprintf("involvedObject.kind=%s", input.Kind)
		}

		var events []corev1.Event
		if input.Namespace != "" {
			evts, err := k8s.CoreV1().Events(input.Namespace).List(ctx, opts)
			if err != nil {
				return errResult("failed to list events: %v", err), nil, nil
			}
			events = evts.Items
		} else {
			evts, err := k8s.CoreV1().Events("").List(ctx, opts)
			if err != nil {
				return errResult("failed to list events: %v", err), nil, nil
			}
			events = evts.Items
		}

		// Sort by last timestamp (most recent first)
		sortEventsByTime(events)

		if len(events) > limit {
			events = events[:limit]
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("Events (%d shown):\n", len(events)))
		lines = append(lines, "LAST SEEN | TYPE | REASON | OBJECT | MESSAGE")
		lines = append(lines, "----------|------|--------|--------|-------")
		for _, e := range events {
			lastSeen := "N/A"
			if !e.LastTimestamp.IsZero() {
				lastSeen = ageStr(e.LastTimestamp) + " ago"
			} else if !e.EventTime.IsZero() {
				lastSeen = ageStr(metav1.Time{Time: e.EventTime.Time}) + " ago"
			}

			object := fmt.Sprintf("%s/%s", e.InvolvedObject.Kind, e.InvolvedObject.Name)
			if e.InvolvedObject.Namespace != "" {
				object = fmt.Sprintf("%s/%s/%s", e.InvolvedObject.Namespace, e.InvolvedObject.Kind, e.InvolvedObject.Name)
			}

			msg := e.Message
			if len(msg) > 120 {
				msg = msg[:117] + "..."
			}

			lines = append(lines, fmt.Sprintf("%s | %s | %s | %s | %s",
				lastSeen, e.Type, e.Reason, object, msg))
		}
		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// list_configmaps (NEW)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_configmaps",
		Description: "[READONLY] List all ConfigMaps in a namespace",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input NamespaceInput) (*mcp.CallToolResult, any, error) {
		ns := nsOrDefault(input.Namespace)
		cms, err := k8s.CoreV1().ConfigMaps(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return errResult("failed to list configmaps: %v", err), nil, nil
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("ConfigMaps in namespace '%s' (%d total):\n", ns, len(cms.Items)))
		lines = append(lines, "NAME | DATA | AGE")
		lines = append(lines, "-----|------|----")
		for _, cm := range cms.Items {
			lines = append(lines, fmt.Sprintf("%s | %d keys | %s",
				cm.Name, len(cm.Data), ageStr(cm.CreationTimestamp)))
		}
		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// get_configmap (NEW)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_configmap",
		Description: "[READONLY] Get detailed ConfigMap data",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ConfigMapInput) (*mcp.CallToolResult, any, error) {
		if input.Name == "" {
			return errResult("name is required"), nil, nil
		}
		ns := nsOrDefault(input.Namespace)
		cm, err := k8s.CoreV1().ConfigMaps(ns).Get(ctx, input.Name, metav1.GetOptions{})
		if err != nil {
			return errResult("failed to get configmap: %v", err), nil, nil
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("ConfigMap: %s/%s\n", ns, cm.Name))

		if len(cm.Data) > 0 {
			lines = append(lines, "Data:")
			for k, v := range cm.Data {
				val := v
				if len(val) > 500 {
					val = val[:497] + "..."
				}
				lines = append(lines, fmt.Sprintf("\n--- %s ---\n%s", k, val))
			}
		}
		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// list_secrets (NEW)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_secrets",
		Description: "[READONLY] List all Secrets in a namespace (keys only, no values)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input NamespaceInput) (*mcp.CallToolResult, any, error) {
		ns := nsOrDefault(input.Namespace)
		secrets, err := k8s.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return errResult("failed to list secrets: %v", err), nil, nil
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("Secrets in namespace '%s' (%d total):\n", ns, len(secrets.Items)))
		lines = append(lines, "NAME | TYPE | DATA | AGE")
		lines = append(lines, "-----|------|------|----")
		for _, s := range secrets.Items {
			keys := make([]string, 0, len(s.Data))
			for k := range s.Data {
				keys = append(keys, k)
			}
			lines = append(lines, fmt.Sprintf("%s | %s | %s | %s",
				s.Name, string(s.Type), strings.Join(keys, ","), ageStr(s.CreationTimestamp)))
		}
		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// get_secret (NEW - values only in dangerous mode)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_secret",
		Description: "[READONLY] Get Secret metadata and keys. Values shown only in dangerous mode with show_values=true",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input SecretInput) (*mcp.CallToolResult, any, error) {
		if input.Name == "" {
			return errResult("name is required"), nil, nil
		}
		ns := nsOrDefault(input.Namespace)
		secret, err := k8s.CoreV1().Secrets(ns).Get(ctx, input.Name, metav1.GetOptions{})
		if err != nil {
			return errResult("failed to get secret: %v", err), nil, nil
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("Secret: %s/%s", ns, secret.Name))
		lines = append(lines, fmt.Sprintf("Type: %s", string(secret.Type)))
		lines = append(lines, fmt.Sprintf("Created: %s (%s ago)", secret.CreationTimestamp.Format(time.RFC3339), ageStr(secret.CreationTimestamp)))

		lines = append(lines, "\nKeys:")
		for k, v := range secret.Data {
			if input.ShowValues && requireDangerous("get_secret") {
				lines = append(lines, fmt.Sprintf("  %s: %s", k, string(v)))
			} else {
				lines = append(lines, fmt.Sprintf("  %s: [%d bytes]", k, len(v)))
			}
		}

		if input.ShowValues && !requireDangerous("get_secret") {
			lines = append(lines, "\n[Values hidden. Use --mode=dangerous to show secret values]")
		}

		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// list_pvc (NEW)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_pvc",
		Description: "[READONLY] List PersistentVolumeClaims in a namespace",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input NamespaceInput) (*mcp.CallToolResult, any, error) {
		ns := nsOrDefault(input.Namespace)
		pvcs, err := k8s.CoreV1().PersistentVolumeClaims(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return errResult("failed to list PVCs: %v", err), nil, nil
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("PersistentVolumeClaims in namespace '%s' (%d total):\n", ns, len(pvcs.Items)))
		lines = append(lines, "NAME | STATUS | VOLUME | CAPACITY | STORAGECLASS | AGE")
		lines = append(lines, "-----|--------|--------|----------|--------------|----")
		for _, pvc := range pvcs.Items {
			capacity := "<pending>"
			if pvc.Status.Capacity != nil {
				if c, ok := pvc.Status.Capacity["storage"]; ok {
					capacity = c.String()
				}
			}
			sc := "<none>"
			if pvc.Spec.StorageClassName != nil {
				sc = *pvc.Spec.StorageClassName
			}
			lines = append(lines, fmt.Sprintf("%s | %s | %s | %s | %s | %s",
				pvc.Name, string(pvc.Status.Phase), pvc.Spec.VolumeName, capacity, sc, ageStr(pvc.CreationTimestamp)))
		}
		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// list_ingress (NEW)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_ingress",
		Description: "[READONLY] List Ingress resources in a namespace",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input NamespaceInput) (*mcp.CallToolResult, any, error) {
		ns := nsOrDefault(input.Namespace)
		ings, err := k8s.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return errResult("failed to list ingress: %v", err), nil, nil
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("Ingresses in namespace '%s' (%d total):\n", ns, len(ings.Items)))
		lines = append(lines, "NAME | HOSTS | ADDRESS | PORTS | AGE")
		lines = append(lines, "-----|------|---------|-------|----")
		for _, ing := range ings.Items {
			var hosts []string
			for _, rule := range ing.Spec.Rules {
				hosts = append(hosts, rule.Host)
			}
			hostStr := "<none>"
			if len(hosts) > 0 {
				hostStr = strings.Join(hosts, ",")
			}

			addr := "<none>"
			if len(ing.Status.LoadBalancer.Ingress) > 0 {
				if ing.Status.LoadBalancer.Ingress[0].IP != "" {
					addr = ing.Status.LoadBalancer.Ingress[0].IP
				} else if ing.Status.LoadBalancer.Ingress[0].Hostname != "" {
					addr = ing.Status.LoadBalancer.Ingress[0].Hostname
				}
			}

			ports := "80"
			var tlsPorts []string
			for _, tls := range ing.Spec.TLS {
				tlsPorts = append(tlsPorts, "443")
				_ = tls
			}
			if len(tlsPorts) > 0 {
				ports = "80,443"
			}

			lines = append(lines, fmt.Sprintf("%s | %s | %s | %s | %s",
				ing.Name, hostStr, addr, ports, ageStr(ing.CreationTimestamp)))
		}
		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// list_jobs (NEW)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_jobs",
		Description: "[READONLY] List Jobs in a namespace",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input NamespaceInput) (*mcp.CallToolResult, any, error) {
		ns := nsOrDefault(input.Namespace)
		jobs, err := k8s.BatchV1().Jobs(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return errResult("failed to list jobs: %v", err), nil, nil
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("Jobs in namespace '%s' (%d total):\n", ns, len(jobs.Items)))
		lines = append(lines, "NAME | COMPLETIONS | DURATION | AGE")
		lines = append(lines, "-----|-------------|----------|----")
		for _, j := range jobs.Items {
			completions := "0/1"
			if j.Spec.Completions != nil {
				completions = fmt.Sprintf("%d/%d", j.Status.Succeeded, *j.Spec.Completions)
			}

			duration := "<running>"
			if j.Status.CompletionTime != nil {
				d := j.Status.CompletionTime.Sub(j.Status.StartTime.Time)
				duration = fmt.Sprintf("%.0fs", d.Seconds())
			}

			lines = append(lines, fmt.Sprintf("%s | %s | %s | %s",
				j.Name, completions, duration, ageStr(j.CreationTimestamp)))
		}
		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// ==================== READWRITE TOOLS ====================

	// restart_deployment
	mcp.AddTool(server, &mcp.Tool{
		Name:        "restart_deployment",
		Description: "[READWRITE] Restart a deployment (kubectl rollout restart). Triggers rolling update.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input RolloutInput) (*mcp.CallToolResult, any, error) {
		if !requireReadWrite("restart_deployment") {
			return permDeniedResult("restart_deployment", ModeReadWrite), nil, nil
		}
		if input.Name == "" {
			return errResult("name is required"), nil, nil
		}
		ns := nsOrDefault(input.Namespace)

		patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, time.Now().Format(time.RFC3339))
		_, err := k8s.AppsV1().Deployments(ns).Patch(ctx, input.Name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
		if err != nil {
			return errResult("failed to restart deployment: %v", err), nil, nil
		}

		return textResult(fmt.Sprintf("Deployment %s/%s restarted successfully. Use rollout_status to monitor progress.", ns, input.Name)), nil, nil
	})

	// restart_statefulset
	mcp.AddTool(server, &mcp.Tool{
		Name:        "restart_statefulset",
		Description: "[READWRITE] Restart a statefulset (kubectl rollout restart). Triggers rolling update.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input RolloutInput) (*mcp.CallToolResult, any, error) {
		if !requireReadWrite("restart_statefulset") {
			return permDeniedResult("restart_statefulset", ModeReadWrite), nil, nil
		}
		if input.Name == "" {
			return errResult("name is required"), nil, nil
		}
		ns := nsOrDefault(input.Namespace)

		patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, time.Now().Format(time.RFC3339))
		_, err := k8s.AppsV1().StatefulSets(ns).Patch(ctx, input.Name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
		if err != nil {
			return errResult("failed to restart statefulset: %v", err), nil, nil
		}

		return textResult(fmt.Sprintf("StatefulSet %s/%s restarted successfully. Use rollout_status to monitor progress.", ns, input.Name)), nil, nil
	})

	// scale_deployment
	mcp.AddTool(server, &mcp.Tool{
		Name:        "scale_deployment",
		Description: "[READWRITE] Scale a deployment to specified number of replicas",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ScaleInput) (*mcp.CallToolResult, any, error) {
		if !requireReadWrite("scale_deployment") {
			return permDeniedResult("scale_deployment", ModeReadWrite), nil, nil
		}
		if input.Name == "" {
			return errResult("name is required"), nil, nil
		}
		ns := nsOrDefault(input.Namespace)

		patch := fmt.Sprintf(`{"spec":{"replicas":%d}}`, input.Replicas)
		_, err := k8s.AppsV1().Deployments(ns).Patch(ctx, input.Name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
		if err != nil {
			return errResult("failed to scale deployment: %v", err), nil, nil
		}

		return textResult(fmt.Sprintf("Deployment %s/%s scaled to %d replicas.", ns, input.Name, input.Replicas)), nil, nil
	})

	// set_image
	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_image",
		Description: "[READWRITE] Update container image in a deployment",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input SetImageInput) (*mcp.CallToolResult, any, error) {
		if !requireReadWrite("set_image") {
			return permDeniedResult("set_image", ModeReadWrite), nil, nil
		}
		if input.Name == "" || input.Container == "" || input.Image == "" {
			return errResult("name, container, and image are all required"), nil, nil
		}
		ns := nsOrDefault(input.Namespace)

		patch := fmt.Sprintf(`{"spec":{"template":{"spec":{"containers":[{"name":"%s","image":"%s"}]}}}}`, input.Container, input.Image)
		_, err := k8s.AppsV1().Deployments(ns).Patch(ctx, input.Name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
		if err != nil {
			return errResult("failed to set image: %v", err), nil, nil
		}

		return textResult(fmt.Sprintf("Deployment %s/%s container '%s' image updated to '%s'.", ns, input.Name, input.Container, input.Image)), nil, nil
	})

	// rollout_status
	mcp.AddTool(server, &mcp.Tool{
		Name:        "rollout_status",
		Description: "[READWRITE] Check rollout status of a deployment",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input RolloutInput) (*mcp.CallToolResult, any, error) {
		if !requireReadWrite("rollout_status") {
			return permDeniedResult("rollout_status", ModeReadWrite), nil, nil
		}
		if input.Name == "" {
			return errResult("name is required"), nil, nil
		}
		ns := nsOrDefault(input.Namespace)

		d, err := k8s.AppsV1().Deployments(ns).Get(ctx, input.Name, metav1.GetOptions{})
		if err != nil {
			return errResult("failed to get deployment: %v", err), nil, nil
		}

		var desired int32
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("Deployment: %s/%s", ns, d.Name))
		lines = append(lines, fmt.Sprintf("Desired: %d | Updated: %d | Ready: %d | Available: %d",
			desired, d.Status.UpdatedReplicas, d.Status.ReadyReplicas, d.Status.AvailableReplicas))

		if d.Status.ReadyReplicas == desired && d.Status.UpdatedReplicas == desired {
			lines = append(lines, "\n✅ Rollout complete!")
		} else {
			lines = append(lines, "\n⏳ Rollout in progress...")
			for _, cond := range d.Status.Conditions {
				lines = append(lines, fmt.Sprintf("  %s: %s - %s", cond.Type, cond.Status, cond.Message))
			}
		}

		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	// create_namespace
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_namespace",
		Description: "[READWRITE] Create a new namespace",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input NamespaceInput) (*mcp.CallToolResult, any, error) {
		if !requireReadWrite("create_namespace") {
			return permDeniedResult("create_namespace", ModeReadWrite), nil, nil
		}
		if input.Namespace == "" {
			return errResult("namespace is required"), nil, nil
		}

		_, err := k8s.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: input.Namespace},
		}, metav1.CreateOptions{})
		if err != nil {
			return errResult("failed to create namespace: %v", err), nil, nil
		}

		return textResult(fmt.Sprintf("Namespace '%s' created successfully.", input.Namespace)), nil, nil
	})

	// patch_deployment
	mcp.AddTool(server, &mcp.Tool{
		Name:        "patch_deployment",
		Description: "[READWRITE] Apply a strategic merge patch to a deployment",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input PatchInput) (*mcp.CallToolResult, any, error) {
		if !requireReadWrite("patch_deployment") {
			return permDeniedResult("patch_deployment", ModeReadWrite), nil, nil
		}
		if input.Name == "" || input.Patch == "" {
			return errResult("name and patch are required"), nil, nil
		}
		ns := nsOrDefault(input.Namespace)

		_, err := k8s.AppsV1().Deployments(ns).Patch(ctx, input.Name, types.StrategicMergePatchType, []byte(input.Patch), metav1.PatchOptions{})
		if err != nil {
			return errResult("failed to patch deployment: %v", err), nil, nil
		}

		return textResult(fmt.Sprintf("Deployment %s/%s patched successfully.", ns, input.Name)), nil, nil
	})

	// ==================== DANGEROUS TOOLS ====================

	// delete_pod
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_pod",
		Description: "[DANGEROUS] Delete a pod. Use with caution!",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input DeletePodInput) (*mcp.CallToolResult, any, error) {
		if !requireDangerous("delete_pod") {
			return permDeniedResult("delete_pod", ModeDangerous), nil, nil
		}
		if input.Name == "" {
			return errResult("name is required"), nil, nil
		}
		ns := nsOrDefault(input.Namespace)

		gracePeriod := int64(30)
		if input.GracePeriod != nil {
			gracePeriod = *input.GracePeriod
		}

		err := k8s.CoreV1().Pods(ns).Delete(ctx, input.Name, metav1.DeleteOptions{
			GracePeriodSeconds: &gracePeriod,
		})
		if err != nil {
			return errResult("failed to delete pod: %v", err), nil, nil
		}

		return textResult(fmt.Sprintf("Pod %s/%s deleted (grace period: %ds).", ns, input.Name, gracePeriod)), nil, nil
	})

	// delete_namespace
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_namespace",
		Description: "[DANGEROUS] Delete a namespace and ALL resources within it!",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input NamespaceInput) (*mcp.CallToolResult, any, error) {
		if !requireDangerous("delete_namespace") {
			return permDeniedResult("delete_namespace", ModeDangerous), nil, nil
		}
		if input.Namespace == "" {
			return errResult("namespace is required"), nil, nil
		}

		err := k8s.CoreV1().Namespaces().Delete(ctx, input.Namespace, metav1.DeleteOptions{})
		if err != nil {
			return errResult("failed to delete namespace: %v", err), nil, nil
		}

		return textResult(fmt.Sprintf("⚠️ Namespace '%s' and ALL its resources are being deleted!", input.Namespace)), nil, nil
	})

	// delete_deployment
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_deployment",
		Description: "[DANGEROUS] Delete a deployment and its pods",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input DeploymentInput) (*mcp.CallToolResult, any, error) {
		if !requireDangerous("delete_deployment") {
			return permDeniedResult("delete_deployment", ModeDangerous), nil, nil
		}
		if input.Name == "" {
			return errResult("name is required"), nil, nil
		}
		ns := nsOrDefault(input.Namespace)

		err := k8s.AppsV1().Deployments(ns).Delete(ctx, input.Name, metav1.DeleteOptions{})
		if err != nil {
			return errResult("failed to delete deployment: %v", err), nil, nil
		}

		return textResult(fmt.Sprintf("⚠️ Deployment %s/%s deleted.", ns, input.Name)), nil, nil
	})

	// apply_yaml
	mcp.AddTool(server, &mcp.Tool{
		Name:        "apply_yaml",
		Description: "[DANGEROUS] Apply arbitrary YAML manifest. Handles single or multi-document YAML.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ApplyYAMLInput) (*mcp.CallToolResult, any, error) {
		if !requireDangerous("apply_yaml") {
			return permDeniedResult("apply_yaml", ModeDangerous), nil, nil
		}
		if input.YAML == "" {
			return errResult("yaml content is required"), nil, nil
		}

		// Note: Full YAML apply would require kubectl runtime or dynamic client.
		// For safety, we return an error suggesting kubectl for now.
		return errResult("apply_yaml is not yet implemented. Use kubectl apply for complex manifests."), nil, nil
	})

	ctx := context.Background()
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}


