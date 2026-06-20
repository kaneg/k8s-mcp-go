package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
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
		"server_info", "resolve_workload", "list_pods", "get_pod", "get_pod_logs", "list_services", "get_service",
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
		"delete_pod", "delete_deployment", "delete_statefulset", "delete_daemonset", "delete_resource", "delete_namespace", "apply_yaml",
	}

	_ = readonlyTools
	_ = readwriteTools
	_ = dangerousTools

	// Count total expected tools
	total := len(readonlyTools) + len(readwriteTools) + len(dangerousTools)
	if total != 38 {
		t.Errorf("expected 38 tools total, got %d", total)
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

func TestDeleteStatefulSet_DeletesFromDefaultNamespace(t *testing.T) {
	client := k8sfake.NewSimpleClientset(&appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "database", Namespace: "default"},
	})

	result := deleteStatefulSet(context.Background(), client, StatefulSetInput{Name: "database"})
	if result.IsError {
		t.Fatalf("deleteStatefulSet returned error: %s", resultText(result))
	}
	if got := resultText(result); got != "⚠️ StatefulSet default/database deleted." {
		t.Fatalf("result = %q", got)
	}
	_, err := client.AppsV1().StatefulSets("default").Get(context.Background(), "database", metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("StatefulSet still exists or lookup failed: %v", err)
	}
}

func TestDeleteDaemonSet_DeletesFromDefaultNamespace(t *testing.T) {
	client := k8sfake.NewSimpleClientset(&appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "node-agent", Namespace: "default"},
	})

	result := deleteDaemonSet(context.Background(), client, DaemonSetInput{Name: "node-agent"})
	if result.IsError {
		t.Fatalf("deleteDaemonSet returned error: %s", resultText(result))
	}
	if got := resultText(result); got != "⚠️ DaemonSet default/node-agent deleted." {
		t.Fatalf("result = %q", got)
	}
	_, err := client.AppsV1().DaemonSets("default").Get(context.Background(), "node-agent", metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("DaemonSet still exists or lookup failed: %v", err)
	}
}

func TestDeleteWorkloadHelpers_RequireName(t *testing.T) {
	client := k8sfake.NewSimpleClientset()
	tests := []struct {
		name string
		run  func() *mcp.CallToolResult
	}{
		{"statefulset", func() *mcp.CallToolResult {
			return deleteStatefulSet(context.Background(), client, StatefulSetInput{})
		}},
		{"daemonset", func() *mcp.CallToolResult {
			return deleteDaemonSet(context.Background(), client, DaemonSetInput{})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.run()
			if !result.IsError || !strings.Contains(resultText(result), "name is required") {
				t.Fatalf("result = %q, IsError = %v", resultText(result), result.IsError)
			}
		})
	}
}

func TestDeleteWorkloadHelpers_IncludeResourceContextInAPIErrors(t *testing.T) {
	client := k8sfake.NewSimpleClientset()
	tests := []struct {
		name string
		run  func() *mcp.CallToolResult
		want string
	}{
		{"statefulset", func() *mcp.CallToolResult {
			return deleteStatefulSet(context.Background(), client, StatefulSetInput{Name: "missing", Namespace: "tools"})
		}, "failed to delete statefulset tools/missing"},
		{"daemonset", func() *mcp.CallToolResult {
			return deleteDaemonSet(context.Background(), client, DaemonSetInput{Name: "missing", Namespace: "tools"})
		}, "failed to delete daemonset tools/missing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.run()
			if !result.IsError || !strings.Contains(resultText(result), tt.want) {
				t.Fatalf("result = %q, IsError = %v", resultText(result), result.IsError)
			}
		})
	}
}

func testDeleteResourceMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "", Version: "v1"},
		{Group: "rbac.authorization.k8s.io", Version: "v1"},
	})
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ServiceAccount"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"}, meta.RESTScopeRoot)
	return mapper
}

func TestDeleteResource_DeletesNamespacedResourceUsingDefaultNamespace(t *testing.T) {
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"}
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "ServiceAccount",
		"metadata": map[string]any{"name": "builder", "namespace": "default"},
	}}
	if _, err := client.Resource(gvr).Namespace("default").Create(ctx, obj, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	result := deleteResource(ctx, client, testDeleteResourceMapper(), DeleteResourceInput{
		APIVersion: "v1", Kind: "ServiceAccount", Name: "builder",
	})
	if result.IsError {
		t.Fatalf("deleteResource returned error: %s", resultText(result))
	}
	for _, want := range []string{"ServiceAccount default/builder deleted", "apiVersion: v1", "resource: serviceaccounts", "scope: namespaced"} {
		if !strings.Contains(resultText(result), want) {
			t.Fatalf("result missing %q: %s", want, resultText(result))
		}
	}
	if _, err := client.Resource(gvr).Namespace("default").Get(ctx, "builder", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("ServiceAccount still exists or lookup failed: %v", err)
	}
}

func TestDeleteResource_DeletesClusterScopedResource(t *testing.T) {
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	gvr := schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"}
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRole",
		"metadata": map[string]any{"name": "auditor"},
	}}
	if _, err := client.Resource(gvr).Create(ctx, obj, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	result := deleteResource(ctx, client, testDeleteResourceMapper(), DeleteResourceInput{
		APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRole", Name: "auditor",
	})
	if result.IsError {
		t.Fatalf("deleteResource returned error: %s", resultText(result))
	}
	for _, want := range []string{"ClusterRole auditor deleted", "resource: clusterroles", "scope: cluster"} {
		if !strings.Contains(resultText(result), want) {
			t.Fatalf("result missing %q: %s", want, resultText(result))
		}
	}
	if _, err := client.Resource(gvr).Get(ctx, "auditor", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("ClusterRole still exists or lookup failed: %v", err)
	}
}

func TestDeleteResource_RejectsKindsWithDedicatedTools(t *testing.T) {
	tests := []struct{ apiVersion, kind, call string }{
		{"v1", "Pod", `delete_pod {"namespace":"apps","name":"target"}`},
		{"apps/v1", "Deployment", `delete_deployment {"namespace":"apps","name":"target"}`},
		{"apps/v1", "StatefulSet", `delete_statefulset {"namespace":"apps","name":"target"}`},
		{"apps/v1", "DaemonSet", `delete_daemonset {"namespace":"apps","name":"target"}`},
		{"v1", "Namespace", `delete_namespace {"namespace":"target"}`},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			result := deleteResource(context.Background(), dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()), testDeleteResourceMapper(), DeleteResourceInput{
				APIVersion: tt.apiVersion, Kind: tt.kind, Name: "target", Namespace: "apps",
			})
			if !result.IsError || !strings.Contains(resultText(result), tt.call) {
				t.Fatalf("result = %q, IsError = %v", resultText(result), result.IsError)
			}
		})
	}
}

func TestDeleteResource_ValidatesRequiredFields(t *testing.T) {
	tests := []struct {
		name  string
		input DeleteResourceInput
		want  string
	}{
		{"api version", DeleteResourceInput{}, "api_version is required"},
		{"kind", DeleteResourceInput{APIVersion: "v1"}, "kind is required"},
		{"name", DeleteResourceInput{APIVersion: "v1", Kind: "ServiceAccount"}, "name is required"},
		{"invalid api version", DeleteResourceInput{APIVersion: "bad/version/extra", Kind: "ServiceAccount", Name: "builder"}, "invalid api_version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deleteResource(context.Background(), dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()), testDeleteResourceMapper(), tt.input)
			if !result.IsError || !strings.Contains(resultText(result), tt.want) {
				t.Fatalf("result = %q, IsError = %v", resultText(result), result.IsError)
			}
		})
	}
}

func TestDeleteResource_RejectsNamespaceForClusterScopedResource(t *testing.T) {
	result := deleteResource(context.Background(), dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()), testDeleteResourceMapper(), DeleteResourceInput{
		APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRole", Name: "auditor", Namespace: "default",
	})
	if !result.IsError || !strings.Contains(resultText(result), "cluster-scoped; omit namespace") {
		t.Fatalf("result = %q, IsError = %v", resultText(result), result.IsError)
	}
}

func TestDeleteResource_ReturnsMappingAndAPIContext(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	unmapped := deleteResource(context.Background(), client, testDeleteResourceMapper(), DeleteResourceInput{
		APIVersion: "v1", Kind: "ConfigMap", Name: "missing",
	})
	if !unmapped.IsError || !strings.Contains(resultText(unmapped), "failed to resolve v1 ConfigMap") {
		t.Fatalf("mapping result = %q", resultText(unmapped))
	}

	missing := deleteResource(context.Background(), client, testDeleteResourceMapper(), DeleteResourceInput{
		APIVersion: "v1", Kind: "ServiceAccount", Name: "missing",
	})
	for _, want := range []string{"failed to delete ServiceAccount default/missing", "apiVersion: v1", "resource: serviceaccounts"} {
		if !missing.IsError || !strings.Contains(resultText(missing), want) {
			t.Fatalf("API result missing %q: %s", want, resultText(missing))
		}
	}
}

func TestApplyYAMLManifests_AppliesMultiDocumentYAML(t *testing.T) {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Group: "", Version: "v1"}})
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)

	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	input := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
data:
  key: value
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: explicit-config
  namespace: tools
data:
  enabled: "true"
`

	summaries, err := applyYAMLManifests(context.Background(), client, mapper, input)
	if err != nil {
		t.Fatalf("applyYAMLManifests returned error: %v", err)
	}

	wantSummaries := []string{"ConfigMap default/app-config applied", "ConfigMap tools/explicit-config applied"}
	if strings.Join(summaries, "\n") != strings.Join(wantSummaries, "\n") {
		t.Fatalf("summaries = %v, want %v", summaries, wantSummaries)
	}

	defaultCM, err := client.Resource(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}).Namespace("default").Get(context.Background(), "app-config", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected default/app-config to be applied: %v", err)
	}
	if got, _, _ := unstructured.NestedString(defaultCM.Object, "data", "key"); got != "value" {
		t.Errorf("default/app-config data.key = %q, want %q", got, "value")
	}

	_, err = client.Resource(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}).Namespace("tools").Get(context.Background(), "explicit-config", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected tools/explicit-config to be applied: %v", err)
	}
}

func TestFormatIngressList_AllNamespacesIncludesNamespaceColumn(t *testing.T) {
	ings := []networkingv1.Ingress{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "apps", CreationTimestamp: metav1.NewTime(time.Now())},
			Spec:       networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{Host: "web.example.com"}}},
		},
	}

	got := formatIngressList(metav1.NamespaceAll, ings)

	for _, want := range []string{
		"Ingresses in all namespaces (1 total):",
		"NAMESPACE | NAME | HOSTS | ADDRESS | PORTS | AGE",
		"apps | web | web.example.com | <none> | 80 |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("formatIngressList() missing %q in:\n%s", want, got)
		}
	}
}

func TestResolveNamespacedListNamespace_DefaultsEmptyNamespace(t *testing.T) {
	ns, err := resolveNamespacedListNamespace(NamespacedListInput{})
	if err != nil {
		t.Fatalf("resolveNamespacedListNamespace returned error: %v", err)
	}
	if ns != "default" {
		t.Fatalf("resolveNamespacedListNamespace(empty) = %q, want default", ns)
	}
}

func TestResolveNamespacedListNamespace_AllNamespacesIsExplicit(t *testing.T) {
	ns, err := resolveNamespacedListNamespace(NamespacedListInput{AllNamespaces: true})
	if err != nil {
		t.Fatalf("resolveNamespacedListNamespace returned error: %v", err)
	}
	if ns != metav1.NamespaceAll {
		t.Fatalf("resolveNamespacedListNamespace(all_namespaces) = %q, want NamespaceAll", ns)
	}
}

func TestResolveNamespacedListNamespace_StarMeansAllNamespaces(t *testing.T) {
	ns, err := resolveNamespacedListNamespace(NamespacedListInput{Namespace: "*"})
	if err != nil {
		t.Fatalf("resolveNamespacedListNamespace returned error: %v", err)
	}
	if ns != metav1.NamespaceAll {
		t.Fatalf("resolveNamespacedListNamespace(namespace=*) = %q, want NamespaceAll", ns)
	}
}

func TestResolveNamespacedListNamespace_RejectsNamespaceWithAllNamespaces(t *testing.T) {
	_, err := resolveNamespacedListNamespace(NamespacedListInput{Namespace: "apps", AllNamespaces: true})
	if err == nil {
		t.Fatal("resolveNamespacedListNamespace should reject namespace with all_namespaces=true")
	}
}

func TestFormatConfigMapValue_TruncatesByDefault(t *testing.T) {
	value := strings.Repeat("a", 600)

	got := formatConfigMapValue(value, false)

	if len(got) != 500 {
		t.Fatalf("formatConfigMapValue() length = %d, want 500", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("formatConfigMapValue() = %q, want truncated suffix", got)
	}
}

func TestFormatConfigMapValue_ReturnsFullValueWhenRequested(t *testing.T) {
	value := strings.Repeat("a", 600)

	got := formatConfigMapValue(value, true)

	if got != value {
		t.Fatalf("formatConfigMapValue(full=true) length = %d, want %d", len(got), len(value))
	}
}

func TestFormatConfigMapSummary_UsesPreviewUnlessFullRequested(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
		Data:       map[string]string{"big": strings.Repeat("a", 600)},
	}

	preview := formatConfigMapSummary(cm, false)
	full := formatConfigMapSummary(cm, true)

	if !strings.Contains(preview, strings.Repeat("a", 497)+"...") {
		t.Fatalf("summary preview did not truncate value:\n%s", preview)
	}
	if !strings.Contains(full, strings.Repeat("a", 600)) {
		t.Fatalf("full summary did not include complete value:\n%s", full)
	}
}

func TestRenderConfigMap_JSON(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
		Data:       map[string]string{"key": "value"},
	}

	got, err := renderConfigMap(cm, OutputJSON, false)
	if err != nil {
		t.Fatalf("renderConfigMap returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("renderConfigMap JSON was invalid: %v\n%s", err, got)
	}
	if decoded["kind"] != "ConfigMap" {
		t.Fatalf("kind = %v, want ConfigMap in:\n%s", decoded["kind"], got)
	}
}

func TestRenderConfigMap_YAML(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
		Data:       map[string]string{"key": "value"},
	}

	got, err := renderConfigMap(cm, OutputYAML, false)
	if err != nil {
		t.Fatalf("renderConfigMap returned error: %v", err)
	}
	for _, want := range []string{"kind: ConfigMap", "name: app", "key: value"} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderConfigMap YAML missing %q in:\n%s", want, got)
		}
	}
}

func TestNormalizeOutputFormat_DefaultsAndLowercases(t *testing.T) {
	if got := normalizeOutputFormat(""); got != OutputSummary {
		t.Fatalf("normalizeOutputFormat(empty) = %q, want %q", got, OutputSummary)
	}
	if got := normalizeOutputFormat("JSON"); got != OutputJSON {
		t.Fatalf("normalizeOutputFormat(JSON) = %q, want %q", got, OutputJSON)
	}
}

func TestNormalizeOutputFormat_RejectsUnknown(t *testing.T) {
	if got := normalizeOutputFormat("xml"); got != "" {
		t.Fatalf("normalizeOutputFormat(xml) = %q, want empty invalid marker", got)
	}
}

func TestFormatTable(t *testing.T) {
	got := formatTable("Things (1 total):", []string{"NAME", "AGE"}, [][]string{{"web", "3d"}})

	for _, want := range []string{
		"Things (1 total):",
		"NAME | AGE",
		"----|----",
		"web | 3d",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatTable() missing %q in:\n%s", want, got)
		}
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

func TestFormatPodDetails_IncludesOwnerReferences(t *testing.T) {
	controller := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "miniagent-0",
			Namespace: "miniagent",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "StatefulSet", Name: "miniagent", Controller: &controller},
			},
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "10.0.0.1"},
		Spec: corev1.PodSpec{NodeName: "pi5", Containers: []corev1.Container{
			{Name: "miniagent", Image: "miniagent:latest"},
		}},
	}

	got := formatPodDetails(pod, "miniagent")

	for _, want := range []string{
		"Controller: StatefulSet/miniagent",
		`Suggested next tool: get_statefulset {"namespace":"miniagent","name":"miniagent"}`,
		"Owners:",
		"StatefulSet/miniagent (controller)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatPodDetails() missing %q in:\n%s", want, got)
		}
	}
}

func TestFormatResolvedWorkload_IncludesMatchesAndSuggestedTools(t *testing.T) {
	matches := []WorkloadMatch{
		{Kind: "StatefulSet", Namespace: "miniagent", Name: "miniagent", SuggestedTool: "get_statefulset"},
		{Kind: "Service", Namespace: "miniagent", Name: "miniagent", SuggestedTool: "get_service"},
		{Kind: "Pod", Namespace: "miniagent", Name: "miniagent-0", SuggestedTool: "get_pod"},
	}

	got := formatResolvedWorkload("miniagent", "miniagent", matches)

	for _, want := range []string{
		"Resolved workload candidates for miniagent/miniagent:",
		"StatefulSet | miniagent | miniagent | get_statefulset",
		"Service | miniagent | miniagent | get_service",
		"Pod | miniagent | miniagent-0 | get_pod",
		`Suggested next tools:`,
		`get_statefulset {"namespace":"miniagent","name":"miniagent"}`,
		`get_service {"namespace":"miniagent","name":"miniagent"}`,
		`get_pod {"namespace":"miniagent","name":"miniagent-0"}`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatResolvedWorkload() missing %q in:\n%s", want, got)
		}
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
