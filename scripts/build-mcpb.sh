#!/usr/bin/env bash
# Build MCPB (MCP Bundle) files for all platforms
# Usage: ./scripts/build-mcpb.sh <version> <dist_dir>
set -euo pipefail

VERSION="${1:?version required (e.g. 0.4.0)}"
DIST_DIR="${2:?dist directory required}"

MCPB_DIR="mcpb-build"
rm -rf "$MCPB_DIR"
mkdir -p "$MCPB_DIR"

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

  # GoReleaser output binary
  if [[ "$platform" == windows_* ]]; then
    BINARY="$DIST_DIR/k8s-mcp-go_${VERSION}_${platform%_*}/k8s-mcp-go.exe"
  else
    BINARY="$DIST_DIR/k8s-mcp-go_${VERSION}_${platform%_*}/k8s-mcp-go"
  fi

  # Handle GoReleaser naming: os_arch -> os/arch
  # GoReleaser creates: dist/k8s-mcp-go_VERSION_linux_amd64_v1/k8s-mcp-go
  OS="${platform%_*}"
  ARCH="${platform#*_}"
  GORELEASER_DIR="$DIST_DIR/k8s-mcp-go_${VERSION}_${OS}_${ARCH}"

  if [[ "$OS" == "windows" ]]; then
    BINARY="$GORELEASER_DIR/k8s-mcp-go.exe"
  else
    BINARY="$GORELEASER_DIR/k8s-mcp-go"
  fi

  if [[ ! -f "$BINARY" ]]; then
    echo "WARN: Binary not found: $BINARY, skipping $platform"
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
    { "name": "describe_pod", "description": "Describe a pod" },
    { "name": "pod_logs", "description": "Get pod logs" },
    { "name": "list_deployments", "description": "List deployments" },
    { "name": "get_deployment", "description": "Get deployment details" },
    { "name": "scale_deployment", "description": "Scale a deployment [readwrite]" },
    { "name": "restart_deployment", "description": "Rolling restart a deployment [readwrite]" },
    { "name": "restart_statefulset", "description": "Rolling restart a statefulset [readwrite]" },
    { "name": "list_services", "description": "List services" },
    { "name": "get_service", "description": "Get service details" },
    { "name": "list_configmaps", "description": "List ConfigMaps" },
    { "name": "get_configmap", "description": "Get ConfigMap data" },
    { "name": "list_nodes", "description": "List cluster nodes" },
    { "name": "get_node", "description": "Get node details" },
    { "name": "list_namespaces", "description": "List namespaces" },
    { "name": "list_events", "description": "List events" },
    { "name": "delete_pod", "description": "Delete a pod [dangerous]" },
    { "name": "delete_deployment", "description": "Delete a deployment [dangerous]" },
    { "name": "delete_service", "description": "Delete a service [dangerous]" },
    { "name": "delete_configmap", "description": "Delete a ConfigMap [dangerous]" },
    { "name": "delete_namespace", "description": "Delete a namespace [dangerous]" },
    { "name": "drain_node", "description": "Drain a node [dangerous]" },
    { "name": "uncordon_node", "description": "Uncordon a node [dangerous]" },
    { "name": "cordon_node", "description": "Cordon a node [dangerous]" },
    { "name": "top_pods", "description": "Get pod resource usage" }
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
done

# Copy all .mcpb files to dist for upload
cp "$MCPB_DIR"/*.mcpb "$DIST_DIR/" 2>/dev/null || true

# Generate SHA-256 hashes
echo ""
echo "=== SHA-256 Hashes ==="
for mcpb in "$MCPB_DIR"/*.mcpb; do
  HASH=$(openssl dgst -sha256 "$mcpb" | awk '{print $2}')
  BASENAME=$(basename "$mcpb")
  echo "$BASENAME: $HASH"
done
