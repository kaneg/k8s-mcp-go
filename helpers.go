package main

import (
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"sigs.k8s.io/yaml"
)

type OutputFormat string

const (
	OutputSummary OutputFormat = "summary"
	OutputJSON    OutputFormat = "json"
	OutputYAML    OutputFormat = "yaml"
)

type WorkloadMatch struct {
	Kind          string
	Namespace     string
	Name          string
	SuggestedTool string
}

// nsOrDefault returns "default" if ns is empty, otherwise returns ns.
func nsOrDefault(ns string) string {
	if ns == "" {
		return "default"
	}
	return ns
}

// ageStr returns a human-readable age string like "5s", "3m", "2h", "7d".
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

func permDeniedResult(tool, required string) *mcp.CallToolResult {
	return errResult("permission denied: tool '%s' requires '%s' mode. Restart with --mode=%s", tool, required, required)
}

func formatServerInfo(mode string, info kubernetesConfigInfo) string {
	return fmt.Sprintf(`Name: k8s-mcp-go
Version: %s
Mode: %s
Config Source: %s
Kubeconfig: %s
Current Context: %s
Cluster: %s
User: %s
API Server: %s
OS: %s
Arch: %s
Go: %s`,
		version,
		mode,
		info.Source,
		info.Kubeconfig,
		info.CurrentContext,
		info.Cluster,
		info.User,
		info.APIServer,
		runtime.GOOS,
		runtime.GOARCH,
		runtime.Version(),
	)
}

func formatConfigMapValue(value string, full bool) string {
	if full || len(value) <= 500 {
		return value
	}
	return value[:497] + "..."
}

func formatConfigMapSummary(cm *corev1.ConfigMap, full bool) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("ConfigMap: %s/%s\n", cm.Namespace, cm.Name))
	if len(cm.Data) > 0 {
		lines = append(lines, "Data:")
		for k, v := range cm.Data {
			lines = append(lines, fmt.Sprintf("\n--- %s ---\n%s", k, formatConfigMapValue(v, full)))
		}
	}
	return strings.Join(lines, "\n")
}

func renderConfigMap(cm *corev1.ConfigMap, output OutputFormat, full bool) (string, error) {
	switch output {
	case OutputSummary:
		return formatConfigMapSummary(cm, full), nil
	case OutputJSON:
		raw := configMapWithTypeMeta(cm)
		b, err := json.MarshalIndent(raw, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	case OutputYAML:
		raw := configMapWithTypeMeta(cm)
		b, err := yaml.Marshal(raw)
		if err != nil {
			return "", err
		}
		return string(b), nil
	default:
		return "", fmt.Errorf("unsupported output format %q", output)
	}
}

func configMapWithTypeMeta(cm *corev1.ConfigMap) *corev1.ConfigMap {
	raw := cm.DeepCopy()
	if raw.APIVersion == "" {
		raw.APIVersion = "v1"
	}
	if raw.Kind == "" {
		raw.Kind = "ConfigMap"
	}
	return raw
}

func normalizeOutputFormat(output string) OutputFormat {
	switch OutputFormat(strings.ToLower(strings.TrimSpace(output))) {
	case "", OutputSummary:
		return OutputSummary
	case OutputJSON:
		return OutputJSON
	case OutputYAML:
		return OutputYAML
	default:
		return ""
	}
}

func formatTable(title string, headers []string, rows [][]string) string {
	var lines []string
	lines = append(lines, title)
	lines = append(lines, strings.Join(headers, " | "))

	separators := make([]string, len(headers))
	for i, header := range headers {
		width := len(header)
		if width < 4 {
			width = 4
		}
		separators[i] = strings.Repeat("-", width)
	}
	lines = append(lines, strings.Join(separators, "|"))

	for _, row := range rows {
		lines = append(lines, strings.Join(row, " | "))
	}
	return strings.Join(lines, "\n")
}

func namespacedListTitle(resource, namespace string, count int) string {
	if namespace == metav1.NamespaceAll {
		return fmt.Sprintf("%s in all namespaces (%d total):", resource, count)
	}
	return fmt.Sprintf("%s in namespace '%s' (%d total):", resource, namespace, count)
}

func namespacedListHeaders(namespace string, headers ...string) []string {
	if namespace == metav1.NamespaceAll {
		return append([]string{"NAMESPACE"}, headers...)
	}
	return headers
}

func namespacedListRow(listNamespace, itemNamespace string, values ...string) []string {
	if listNamespace == metav1.NamespaceAll {
		return append([]string{itemNamespace}, values...)
	}
	return values
}

func resolveNamespacedListNamespace(input NamespacedListInput) (string, error) {
	ns := strings.TrimSpace(input.Namespace)
	if input.AllNamespaces {
		if ns != "" && ns != "*" {
			return "", fmt.Errorf("namespace cannot be set when all_namespaces=true")
		}
		return metav1.NamespaceAll, nil
	}
	if ns == "*" {
		return metav1.NamespaceAll, nil
	}
	return nsOrDefault(input.Namespace), nil
}

// checkPermission checks if a tool is allowed in the given mode.
// Returns true if allowed, false if denied.
func checkPermission(currentMode, requiredMode string) bool {
	modeRank := map[string]int{
		ModeReadOnly:  0,
		ModeReadWrite: 1,
		ModeDangerous: 2,
	}
	return modeRank[currentMode] >= modeRank[requiredMode]
}

func shouldRegisterTool(currentMode, requiredMode string) bool {
	return checkPermission(currentMode, requiredMode)
}

func sortEventsByTime(events []corev1.Event) {
	for i := 1; i < len(events); i++ {
		for j := i; j > 0; j-- {
			prev := events[j-1].LastTimestamp.Time
			if events[j-1].LastTimestamp.IsZero() {
				prev = events[j-1].EventTime.Time
			}
			curr := events[j].LastTimestamp.Time
			if events[j].LastTimestamp.IsZero() {
				curr = events[j].EventTime.Time
			}
			if curr.After(prev) {
				events[j-1], events[j] = events[j], events[j-1]
			} else {
				break
			}
		}
	}
}

// helper to extract text from CallToolResult
func resultText(r *mcp.CallToolResult) string {
	if len(r.Content) == 0 {
		return ""
	}
	if tc, ok := r.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

func formatPodDetails(pod *corev1.Pod, namespace string) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("Pod: %s/%s", namespace, pod.Name))
	lines = append(lines, fmt.Sprintf("Status: %s", string(pod.Status.Phase)))
	lines = append(lines, fmt.Sprintf("Node: %s", pod.Spec.NodeName))
	lines = append(lines, fmt.Sprintf("IP: %s", pod.Status.PodIP))
	if controller := controllerOwner(pod.OwnerReferences); controller != "" {
		lines = append(lines, fmt.Sprintf("Controller: %s", controller))
		if suggestion := controllerNextTool(namespace, controller); suggestion != "" {
			lines = append(lines, suggestion)
		}
	}
	lines = append(lines, fmt.Sprintf("Created: %s (%s ago)", pod.CreationTimestamp.Format(time.RFC3339), ageStr(pod.CreationTimestamp)))

	if len(pod.OwnerReferences) > 0 {
		lines = append(lines, "\nOwners:")
		for _, owner := range pod.OwnerReferences {
			suffix := ""
			if owner.Controller != nil && *owner.Controller {
				suffix = " (controller)"
			}
			lines = append(lines, fmt.Sprintf("  %s/%s%s", owner.Kind, owner.Name, suffix))
		}
	}

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
	return strings.Join(lines, "\n")
}

func controllerOwner(owners []metav1.OwnerReference) string {
	for _, owner := range owners {
		if owner.Controller != nil && *owner.Controller {
			return fmt.Sprintf("%s/%s", owner.Kind, owner.Name)
		}
	}
	return ""
}

func controllerNextTool(namespace, controller string) string {
	parts := strings.SplitN(controller, "/", 2)
	if len(parts) != 2 {
		return ""
	}

	tool := ""
	switch parts[0] {
	case "Deployment":
		tool = "get_deployment"
	case "StatefulSet":
		tool = "get_statefulset"
	default:
		return ""
	}

	return fmt.Sprintf(`Suggested next tool: %s {"namespace":"%s","name":"%s"}`, tool, namespace, parts[1])
}

func formatResolvedWorkload(namespace, name string, matches []WorkloadMatch) string {
	if len(matches) == 0 {
		return fmt.Sprintf("No workload candidates found for %s/%s.", namespace, name)
	}

	rows := make([][]string, 0, len(matches))
	for _, match := range matches {
		rows = append(rows, []string{match.Kind, match.Namespace, match.Name, match.SuggestedTool})
	}

	lines := []string{
		formatTable(
			fmt.Sprintf("Resolved workload candidates for %s/%s:", namespace, name),
			[]string{"KIND", "NAMESPACE", "NAME", "SUGGESTED TOOL"},
			rows,
		),
		"\nSuggested next tools:",
	}
	for _, match := range matches {
		if match.SuggestedTool == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf(`%s {"namespace":"%s","name":"%s"}`, match.SuggestedTool, match.Namespace, match.Name))
	}
	return strings.Join(lines, "\n")
}

// normalizeTableLines trims and filters empty lines for comparison
func normalizeTableLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

func formatCPUUsage(q resource.Quantity) string {
	return fmt.Sprintf("%dm", q.MilliValue())
}

func formatMemoryUsage(q resource.Quantity) string {
	bytes := q.Value()
	gi := float64(bytes) / float64(1024*1024*1024)
	if math.Abs(gi-math.Round(gi)) < 0.05 {
		return fmt.Sprintf("%.1fGi", gi)
	}
	if gi >= 1 {
		return fmt.Sprintf("%.1fGi", gi)
	}
	mi := bytes / (1024 * 1024)
	return fmt.Sprintf("%dMi", mi)
}

func sumPodUsage(pod metricsv1beta1.PodMetrics) (resource.Quantity, resource.Quantity) {
	totalCPU := resource.MustParse("0")
	totalMem := resource.MustParse("0")
	for _, container := range pod.Containers {
		if cpu, ok := container.Usage[corev1.ResourceCPU]; ok {
			totalCPU.Add(cpu)
		}
		if mem, ok := container.Usage[corev1.ResourceMemory]; ok {
			totalMem.Add(mem)
		}
	}
	return totalCPU, totalMem
}

func formatIngressList(namespace string, ingresses []networkingv1.Ingress) string {
	sort.Slice(ingresses, func(i, j int) bool {
		left := ingresses[i].Namespace + "/" + ingresses[i].Name
		right := ingresses[j].Namespace + "/" + ingresses[j].Name
		return left < right
	})

	allNamespaces := namespace == metav1.NamespaceAll
	scope := fmt.Sprintf("namespace '%s'", namespace)
	if allNamespaces {
		scope = "all namespaces"
	}

	headers := []string{"NAME", "HOSTS", "ADDRESS", "PORTS", "AGE"}
	if allNamespaces {
		headers = append([]string{"NAMESPACE"}, headers...)
	}

	var rows [][]string
	for _, ing := range ingresses {
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
		if len(ing.Spec.TLS) > 0 {
			ports = "80,443"
		}

		values := []string{ing.Name, hostStr, addr, ports, ageStr(ing.CreationTimestamp)}
		if allNamespaces {
			values = append([]string{ing.Namespace}, values...)
		}
		rows = append(rows, values)
	}

	return formatTable(fmt.Sprintf("Ingresses in %s (%d total):", scope, len(ingresses)), headers, rows)
}
