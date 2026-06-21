package main

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestPackagedToolNamesFollowVerbFirstContract(t *testing.T) {
	names := packagedToolNames(t)
	allowedPrefixes := []string{
		"get_", "list_", "resolve_", "top_", "restart_", "scale_", "set_",
		"create_", "patch_", "delete_", "apply_",
	}

	for _, name := range names {
		valid := false
		for _, prefix := range allowedPrefixes {
			if strings.HasPrefix(name, prefix) {
				valid = true
				break
			}
		}
		if !valid {
			t.Errorf("tool %q must start with an approved action verb", name)
		}
	}

	for oldName, newName := range map[string]string{
		"server_info":      "get_server_info",
		"cluster_overview": "get_cluster_overview",
		"get_events":       "list_events",
		"rollout_status":   "get_rollout_status",
	} {
		if containsString(names, oldName) {
			t.Errorf("deprecated tool %q is still exposed; use %q", oldName, newName)
		}
		if !containsString(names, newName) {
			t.Errorf("renamed tool %q is not exposed", newName)
		}
	}
}

func TestPackagedToolNamesMatchRegistrations(t *testing.T) {
	manifestNames := packagedToolNames(t)
	mainSource, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`(?s)mcp\.Tool\{\s*Name:\s*"([^"]+)"`)
	matches := pattern.FindAllSubmatch(mainSource, -1)
	registeredNames := make([]string, 0, len(matches))
	for _, match := range matches {
		registeredNames = append(registeredNames, string(match[1]))
	}
	sort.Strings(registeredNames)
	sort.Strings(manifestNames)

	if strings.Join(registeredNames, "\n") != strings.Join(manifestNames, "\n") {
		t.Errorf("registered tools and packaged manifest differ\nregistered: %v\nmanifest: %v", registeredNames, manifestNames)
	}
}

func packagedToolNames(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("mcpb/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(manifest.Tools))
	for _, tool := range manifest.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
