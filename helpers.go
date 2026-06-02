package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

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
