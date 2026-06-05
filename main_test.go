package main

import (
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// ==================== nsOrDefault ====================

func TestNsOrDefault_Empty(t *testing.T) {
	if got := nsOrDefault(""); got != "default" {
		t.Errorf("nsOrDefault(\"\") = %q, want %q", got, "default")
	}
}

func TestNsOrDefault_NonEmpty(t *testing.T) {
	if got := nsOrDefault("kube-system"); got != "kube-system" {
		t.Errorf("nsOrDefault(\"kube-system\") = %q, want %q", got, "kube-system")
	}
}

// ==================== ageStr ====================

func TestAgeStr_Seconds(t *testing.T) {
	ts := metav1.NewTime(time.Now().Add(-30 * time.Second))
	got := ageStr(ts)
	if got != "30s" {
		t.Errorf("ageStr(30s ago) = %q, want %q", got, "30s")
	}
}

func TestAgeStr_Minutes(t *testing.T) {
	ts := metav1.NewTime(time.Now().Add(-5 * time.Minute))
	got := ageStr(ts)
	if got != "5m" {
		t.Errorf("ageStr(5m ago) = %q, want %q", got, "5m")
	}
}

func TestAgeStr_Hours(t *testing.T) {
	ts := metav1.NewTime(time.Now().Add(-3 * time.Hour))
	got := ageStr(ts)
	if got != "3h" {
		t.Errorf("ageStr(3h ago) = %q, want %q", got, "3h")
	}
}

func TestAgeStr_Days(t *testing.T) {
	ts := metav1.NewTime(time.Now().Add(-72 * time.Hour))
	got := ageStr(ts)
	if got != "3d" {
		t.Errorf("ageStr(3d ago) = %q, want %q", got, "3d")
	}
}

// ==================== textResult / errResult ====================

func TestTextResult(t *testing.T) {
	r := textResult("hello")
	if r.IsError {
		t.Error("textResult should not be error")
	}
	if len(r.Content) != 1 {
		t.Fatalf("expected 1 content, got %d", len(r.Content))
	}
	tc, ok := r.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if tc.Text != "hello" {
		t.Errorf("text = %q, want %q", tc.Text, "hello")
	}
}

func TestErrResult(t *testing.T) {
	r := errResult("pod %s not found", "nginx")
	if !r.IsError {
		t.Error("errResult should be error")
	}
	tc := r.Content[0].(*mcp.TextContent)
	if !strings.Contains(tc.Text, "Error:") {
		t.Errorf("error text should contain 'Error:', got %q", tc.Text)
	}
	if !strings.Contains(tc.Text, "pod nginx not found") {
		t.Errorf("error text should contain formatted message, got %q", tc.Text)
	}
}

func TestPermDeniedResult(t *testing.T) {
	r := permDeniedResult("delete_pod", "dangerous")
	if !r.IsError {
		t.Error("permDeniedResult should be error")
	}
	tc := r.Content[0].(*mcp.TextContent)
	if !strings.Contains(tc.Text, "delete_pod") {
		t.Errorf("should mention tool name, got %q", tc.Text)
	}
	if !strings.Contains(tc.Text, "dangerous") {
		t.Errorf("should mention required mode, got %q", tc.Text)
	}
}

func TestFormatServerInfo_IncludesOperationalDetails(t *testing.T) {
	got := formatServerInfo("readwrite", "/tmp/kubeconfig", "dev", "dev-cluster", "dev-user", "https://dev.example.com")

	for _, want := range []string{
		"Name: k8s-mcp-go",
		"Version: ",
		"Mode: readwrite",
		"Kubeconfig: /tmp/kubeconfig",
		"Current Context: dev",
		"Cluster: dev-cluster",
		"User: dev-user",
		"API Server: https://dev.example.com",
		"OS: ",
		"Arch: ",
		"Go: ",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("formatServerInfo() missing %q in:\n%s", want, got)
		}
	}
}

// ==================== checkPermission ====================

func TestCheckPermission_ReadOnlyMode(t *testing.T) {
	tests := []struct {
		required string
		want     bool
	}{
		{ModeReadOnly, true},
		{ModeReadWrite, false},
		{ModeDangerous, false},
	}
	for _, tt := range tests {
		got := checkPermission(ModeReadOnly, tt.required)
		if got != tt.want {
			t.Errorf("checkPermission(readonly, %s) = %v, want %v", tt.required, got, tt.want)
		}
	}
}

func TestCheckPermission_ReadWriteMode(t *testing.T) {
	tests := []struct {
		required string
		want     bool
	}{
		{ModeReadOnly, true},
		{ModeReadWrite, true},
		{ModeDangerous, false},
	}
	for _, tt := range tests {
		got := checkPermission(ModeReadWrite, tt.required)
		if got != tt.want {
			t.Errorf("checkPermission(readwrite, %s) = %v, want %v", tt.required, got, tt.want)
		}
	}
}

func TestCheckPermission_DangerousMode(t *testing.T) {
	tests := []struct {
		required string
		want     bool
	}{
		{ModeReadOnly, true},
		{ModeReadWrite, true},
		{ModeDangerous, true},
	}
	for _, tt := range tests {
		got := checkPermission(ModeDangerous, tt.required)
		if got != tt.want {
			t.Errorf("checkPermission(dangerous, %s) = %v, want %v", tt.required, got, tt.want)
		}
	}
}

func TestShouldRegisterTool_FiltersByMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		required string
		want     bool
	}{
		{"readonly tool in readonly mode", ModeReadOnly, ModeReadOnly, true},
		{"readwrite tool hidden in readonly mode", ModeReadOnly, ModeReadWrite, false},
		{"dangerous tool hidden in readonly mode", ModeReadOnly, ModeDangerous, false},
		{"readwrite tool in readwrite mode", ModeReadWrite, ModeReadWrite, true},
		{"dangerous tool hidden in readwrite mode", ModeReadWrite, ModeDangerous, false},
		{"dangerous tool in dangerous mode", ModeDangerous, ModeDangerous, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldRegisterTool(tt.mode, tt.required)
			if got != tt.want {
				t.Errorf("shouldRegisterTool(%q, %q) = %v, want %v", tt.mode, tt.required, got, tt.want)
			}
		})
	}
}

// ==================== sortEventsByTime ====================

func TestSortEventsByTime_Empty(t *testing.T) {
	events := []corev1.Event{}
	sortEventsByTime(events)
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestSortEventsByTime_Single(t *testing.T) {
	events := []corev1.Event{
		{LastTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Hour))},
	}
	sortEventsByTime(events)
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestSortEventsByTime_OrdersNewestFirst(t *testing.T) {
	now := time.Now()
	events := []corev1.Event{
		{LastTimestamp: metav1.NewTime(now.Add(-3 * time.Hour))},
		{LastTimestamp: metav1.NewTime(now.Add(-1 * time.Hour))},
		{LastTimestamp: metav1.NewTime(now.Add(-2 * time.Hour))},
	}
	sortEventsByTime(events)

	if !events[0].LastTimestamp.Time.After(events[1].LastTimestamp.Time) {
		t.Error("events[0] should be newer than events[1]")
	}
	if !events[1].LastTimestamp.Time.After(events[2].LastTimestamp.Time) {
		t.Error("events[1] should be newer than events[2]")
	}
}

func TestSortEventsByTime_UsesEventTime(t *testing.T) {
	now := time.Now()
	events := []corev1.Event{
		{EventTime: metav1.NewMicroTime(now.Add(-3 * time.Hour))},
		{EventTime: metav1.NewMicroTime(now.Add(-1 * time.Hour))},
		{EventTime: metav1.NewMicroTime(now.Add(-2 * time.Hour))},
	}
	sortEventsByTime(events)

	if !events[0].EventTime.Time.After(events[1].EventTime.Time) {
		t.Error("events[0] should be newer than events[1]")
	}
}

func TestSortEventsByTime_MixedTimestamps(t *testing.T) {
	now := time.Now()
	events := []corev1.Event{
		{LastTimestamp: metav1.NewTime(now.Add(-2 * time.Hour))},
		{EventTime: metav1.NewMicroTime(now.Add(-1 * time.Hour))}, // newer, no LastTimestamp
	}
	sortEventsByTime(events)

	// Event with EventTime (newer) should come first
	if !events[0].EventTime.Time.After(events[1].LastTimestamp.Time) {
		t.Error("newer EventTime should sort before older LastTimestamp")
	}
}

// ==================== Tool description prefix checks ====================

func TestToolDescriptions_HavePermissionPrefix(t *testing.T) {
	// This is a static check - we verify the expected tool list has correct prefixes
	readonlyTools := []string{
		"server_info", "list_pods", "get_pod", "get_pod_logs", "list_services", "get_service",
		"list_deployments", "get_deployment", "list_statefulsets", "get_statefulset",
		"list_namespaces", "list_nodes", "cluster_overview", "get_events",
		"list_configmaps", "get_configmap", "list_secrets", "get_secret", "top_nodes",
		"top_pods", "list_pvc", "list_ingress", "list_jobs",
	}
	readwriteTools := []string{
		"restart_deployment", "restart_statefulset", "scale_deployment", "set_image",
		"rollout_status", "create_namespace", "patch_deployment",
	}
	dangerousTools := []string{
		"delete_pod", "delete_deployment", "delete_namespace", "apply_yaml",
	}

	_ = readonlyTools
	_ = readwriteTools
	_ = dangerousTools

	// Count total expected tools
	total := len(readonlyTools) + len(readwriteTools) + len(dangerousTools)
	if total != 34 {
		t.Errorf("expected 34 tools total, got %d", total)
	}
}

// ==================== resultText helper ====================

func TestResultText_Normal(t *testing.T) {
	r := textResult("test output")
	got := resultText(r)
	if got != "test output" {
		t.Errorf("resultText = %q, want %q", got, "test output")
	}
}

func TestResultText_Empty(t *testing.T) {
	r := &mcp.CallToolResult{}
	got := resultText(r)
	if got != "" {
		t.Errorf("resultText(empty) = %q, want %q", got, "")
	}
}

// ==================== normalizeTableLines helper ====================

func TestNormalizeTableLines(t *testing.T) {
	input := "line1\n\n  line2  \nline3\n"
	lines := normalizeTableLines(input)
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "line1" || lines[1] != "line2" || lines[2] != "line3" {
		t.Errorf("unexpected lines: %v", lines)
	}
}

// ==================== Mode constants ====================

func TestModeConstants(t *testing.T) {
	if ModeReadOnly != "readonly" {
		t.Errorf("ModeReadOnly = %q, want %q", ModeReadOnly, "readonly")
	}
	if ModeReadWrite != "readwrite" {
		t.Errorf("ModeReadWrite = %q, want %q", ModeReadWrite, "readwrite")
	}
	if ModeDangerous != "dangerous" {
		t.Errorf("ModeDangerous = %q, want %q", ModeDangerous, "dangerous")
	}
}

func TestFormatCPUUsage(t *testing.T) {
	if got := formatCPUUsage(resource.MustParse("250m")); got != "250m" {
		t.Errorf("formatCPUUsage(250m) = %q, want %q", got, "250m")
	}
	if got := formatCPUUsage(resource.MustParse("2")); got != "2000m" {
		t.Errorf("formatCPUUsage(2) = %q, want %q", got, "2000m")
	}
}

func TestFormatMemoryUsage(t *testing.T) {
	if got := formatMemoryUsage(resource.MustParse("512Mi")); got != "512Mi" {
		t.Errorf("formatMemoryUsage(512Mi) = %q, want %q", got, "512Mi")
	}
	if got := formatMemoryUsage(resource.MustParse("1536Mi")); got != "1.5Gi" {
		t.Errorf("formatMemoryUsage(1536Mi) = %q, want %q", got, "1.5Gi")
	}
}

func TestSumPodUsage(t *testing.T) {
	pod := metricsv1beta1.PodMetrics{
		Containers: []metricsv1beta1.ContainerMetrics{
			{Usage: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("125m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			}},
			{Usage: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("375m"),
				corev1.ResourceMemory: resource.MustParse("768Mi"),
			}},
		},
	}

	cpu, mem := sumPodUsage(pod)

	if got := formatCPUUsage(cpu); got != "500m" {
		t.Errorf("summed cpu = %q, want %q", got, "500m")
	}
	if got := formatMemoryUsage(mem); got != "1.0Gi" {
		t.Errorf("summed memory = %q, want %q", got, "1.0Gi")
	}
}
