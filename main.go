package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var version = "dev"

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
}

type ServiceInput struct {
	Name      string `json:"name" jsonschema:"Service name"`
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace (default: default)"`
}

type DeploymentInput struct {
	Name      string `json:"name" jsonschema:"Deployment name"`
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace (default: default)"`
}

type EmptyInput struct{}

// --- Helpers ---

func nsOrDefault(ns string) string {
	if ns == "" {
		return "default"
	}
	return ns
}

func ageStr(t metav1.Time) string {
	d := time.Since(t.Time)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

func errResult(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: "+format, args...)}},
		IsError: true,
	}
}

// --- Main ---

func main() {
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

	logger.Info("k8s-mcp-go starting", "version", version, "kubeconfig", kubeconfig)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "k8s-mcp-go",
		Version: version,
	}, nil)

	// --- Register tools ---

	// list_pods
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_pods",
		Description: "List all pods in a Kubernetes namespace with status, readiness, restarts, and age",
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
		Description: "Get detailed information about a specific pod including containers, status, and conditions",
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
		Description: "Get logs from a pod. Supports container selection and tail lines",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input LogsInput) (*mcp.CallToolResult, any, error) {
		if input.Name == "" {
			return errResult("name is required"), nil, nil
		}
		ns := nsOrDefault(input.Namespace)
		tail := int64(100)
		if input.TailLines > 0 {
			tail = int64(input.TailLines)
		}

		opts := &corev1.PodLogOptions{TailLines: &tail}
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

		header := fmt.Sprintf("Logs for pod %s/%s (last %d lines):\n\n", ns, input.Name, tail)
		return textResult(header + sb.String()), nil, nil
	})

	// list_services
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_services",
		Description: "List all services in a namespace with type, cluster IP, external IP, and ports",
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
		Description: "Get detailed information about a service including ports, selectors, and endpoints",
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
		Description: "List all deployments in a namespace with replica status",
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
		Description: "Get detailed deployment info including replica status, strategy, containers, and conditions",
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

	// list_namespaces
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_namespaces",
		Description: "List all Kubernetes namespaces with status and age",
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
		Description: "List all cluster nodes with status, roles, version, and OS info",
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
		Description: "Get a high-level cluster overview: node health, pod counts, resource summary, and problem pods",
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

	ctx := context.Background()
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
