#!/usr/bin/env bash
# Build MCPB (MCP Bundle) files for all platforms
# Usage: ./scripts/build-mcpb.sh <version> <dist_dir>
set -euo pipefail

VERSION="${1:?version required (e.g. 0.4.0)}"
DIST_DIR="${2:?dist directory required}"

MCPB_DIR="mcpb-build"
rm -rf "$MCPB_DIR"
mkdir -p "$MCPB_DIR"
built_count=0

find_binary() {
  local os="$1"
  local arch="$2"
  local bin_name="$3"
  local pattern candidate

  for pattern in \
    "$DIST_DIR/k8s-mcp-go_${os}_${arch}"*/"$bin_name" \
    "$DIST_DIR/k8s-mcp-go_${VERSION}_${os}_${arch}"*/"$bin_name"; do
    while IFS= read -r candidate; do
      if [[ -f "$candidate" ]]; then
        printf '%s\n' "$candidate"
        return 0
      fi
    done < <(compgen -G "$pattern" | sort)
  done

  return 1
}

sha256_file() {
  local file="$1"

  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
  else
    openssl dgst -sha256 "$file" | awk '{print $2}'
  fi
}

# Map GoReleaser binary paths to MCPB bundles
platforms=(
  "linux_amd64:k8s-mcp-go:server/k8s-mcp-go"
  "linux_arm64:k8s-mcp-go:server/k8s-mcp-go"
  "darwin_amd64:k8s-mcp-go:server/k8s-mcp-go"
  "darwin_arm64:k8s-mcp-go:server/k8s-mcp-go"
  "windows_amd64:k8s-mcp-go.exe:server/k8s-mcp-go.exe"
  "windows_arm64:k8s-mcp-go.exe:server/k8s-mcp-go.exe"
)

for entry in "${platforms[@]}"; do
  IFS=':' read -r platform bin_name mcpb_path <<< "$entry"

  OS="${platform%_*}"
  ARCH="${platform#*_}"

  if ! BINARY="$(find_binary "$OS" "$ARCH" "$bin_name")"; then
    echo "WARN: Binary not found for $platform, skipping"
    continue
  fi

  # Create MCPB directory structure
  WORK="$MCPB_DIR/$platform"
  mkdir -p "$WORK/server"
  cp "$BINARY" "$WORK/$mcpb_path"
  chmod +x "$WORK/$mcpb_path"

  # Generate manifest.json for this platform
  if [[ "$OS" == "windows" ]]; then
    CMD='${__dirname}\\server\\k8s-mcp-go.exe'
  else
    CMD='${__dirname}/server/k8s-mcp-go'
  fi

  cat > "$WORK/manifest.json" <<EOF
{
  "manifest_version": "0.3",
  "name": "k8s-mcp-go",
  "display_name": "K8s MCP Server (Go)",
  "version": "$VERSION",
  "description": "Safe, read-only-by-default Kubernetes access for AI agents with permission modes.",
  "author": {
    "name": "Kane",
    "email": "gzkaneg@gmail.com"
  },
  "license": "MIT",
  "repository": {
    "type": "git",
    "url": "https://github.com/kaneg/k8s-mcp-go.git"
  },
  "homepage": "https://github.com/kaneg/k8s-mcp-go",
  "support": "https://github.com/kaneg/k8s-mcp-go/issues",
  "server": {
    "type": "binary",
    "entry_point": "$mcpb_path",
    "mcp_config": {
      "command": "$CMD",
      "args": ["-mode", "\${user_config.mode}"],
      "env": {}
    }
  },
  "tools": [
    { "name": "list_pods", "description": "List pods in a namespace" },
    { "name": "get_pod", "description": "Get detailed pod information" },
    { "name": "get_pod_logs", "description": "Get pod logs" },
    { "name": "list_deployments", "description": "List deployments" },
    { "name": "get_deployment", "description": "Get deployment details" },
    { "name": "list_statefulsets", "description": "List StatefulSets" },
    { "name": "get_statefulset", "description": "Get StatefulSet details" },
    { "name": "list_services", "description": "List services" },
    { "name": "get_service", "description": "Get service details" },
    { "name": "list_configmaps", "description": "List ConfigMaps" },
    { "name": "get_configmap", "description": "Get ConfigMap data" },
    { "name": "list_secrets", "description": "List Secrets (keys only)" },
    { "name": "get_secret", "description": "Get Secret metadata and keys" },
    { "name": "list_pvc", "description": "List PersistentVolumeClaims" },
    { "name": "list_ingress", "description": "List Ingress resources" },
    { "name": "list_jobs", "description": "List Jobs" },
    { "name": "top_nodes", "description": "Get current node CPU and memory usage" },
    { "name": "top_pods", "description": "Get current pod CPU and memory usage" },
    { "name": "list_nodes", "description": "List cluster nodes" },
    { "name": "list_namespaces", "description": "List namespaces" },
    { "name": "cluster_overview", "description": "Get cluster health summary" },
    { "name": "get_events", "description": "List events" },
    { "name": "restart_deployment", "description": "Rolling restart a deployment [readwrite]" },
    { "name": "restart_statefulset", "description": "Rolling restart a statefulset [readwrite]" },
    { "name": "scale_deployment", "description": "Scale a deployment [readwrite]" },
    { "name": "set_image", "description": "Update container image [readwrite]" },
    { "name": "rollout_status", "description": "Check rollout status [readwrite]" },
    { "name": "create_namespace", "description": "Create a namespace [readwrite]" },
    { "name": "patch_deployment", "description": "Patch a deployment [readwrite]" },
    { "name": "delete_pod", "description": "Delete a pod [dangerous]" },
    { "name": "delete_deployment", "description": "Delete a deployment [dangerous]" },
    { "name": "delete_namespace", "description": "Delete a namespace [dangerous]" },
    { "name": "apply_yaml", "description": "Apply arbitrary YAML manifest [dangerous]" }
  ],
  "tools_generated": false,
  "keywords": ["kubernetes", "k8s", "mcp", "devops", "cluster", "kubectl"],
  "compatibility": {
    "platforms": ["$([[ "$OS" == "darwin" ]] && echo "darwin" || echo "$OS")"]
  },
  "user_config": {
    "mode": {
      "type": "string",
      "title": "Permission Mode",
      "description": "Permission mode: readonly (default), readwrite, dangerous",
      "default": "readonly",
      "required": false
    }
  }
}
EOF

  # Create .mcpb (ZIP) file
  MCPB_FILE="$MCPB_DIR/k8s-mcp-go_${VERSION}_${platform}.mcpb"
  (cd "$WORK" && zip -r "../../$MCPB_FILE" .)
  echo "Built: $MCPB_FILE ($(du -h "$MCPB_FILE" | cut -f1))"
  built_count=$((built_count + 1))
done

if [[ "$built_count" -eq 0 ]]; then
  echo "ERROR: no MCPB bundles were built from $DIST_DIR" >&2
  exit 1
fi

# Copy all .mcpb files to dist for upload
mcpb_files=("$MCPB_DIR"/*.mcpb)
cp "${mcpb_files[@]}" "$DIST_DIR/"

# Generate SHA-256 hashes
echo ""
echo "=== SHA-256 Hashes ==="
for mcpb in "${mcpb_files[@]}"; do
  HASH=$(sha256_file "$mcpb")
  BASENAME=$(basename "$mcpb")
  echo "$BASENAME: $HASH"
done
