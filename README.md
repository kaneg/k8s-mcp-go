# k8s-mcp-go

A high-performance Kubernetes MCP Server written in Go. Bridges AI agents (Claude, Cursor, Codex, etc.) to your K8s cluster.

## Features

- **25 MCP Tools** — read, write, and dangerous operations
- **3-tier permission model** — readonly (default), readwrite, dangerous
- **Zero configuration** — auto-detects `~/.kube/config`
- **Single binary** — no dependencies, no runtime
- **Fast** — Go implementation, instant startup

## Quick Start

### 1. Download

Download the latest binary from [Releases](https://github.com/kaneg/k8s-mcp-go/releases/latest).

| OS | Arch | File |
|----|------|------|
| Linux | x86_64 | `k8s-mcp-go_*_linux_amd64.tar.gz` |
| Linux | ARM64 | `k8s-mcp-go_*_linux_arm64.tar.gz` |
| macOS | Intel | `k8s-mcp-go_*_darwin_amd64.tar.gz` |
| macOS | Apple Silicon | `k8s-mcp-go_*_darwin_arm64.tar.gz` |
| Windows | x86_64 | `k8s-mcp-go_*_windows_amd64.zip` |

```bash
# Example: Linux x86_64
tar xzf k8s-mcp-go_*_linux_amd64.tar.gz
chmod +x k8s-mcp-go
sudo mv k8s-mcp-go /usr/local/bin/
```

### 2. Run

```bash
# Readonly mode (default, safest)
k8s-mcp-go

# Readwrite mode (enables scale, restart, update image)
k8s-mcp-go --mode=readwrite

# Dangerous mode (enables delete, apply)
k8s-mcp-go --mode=dangerous
```

### 3. Connect to your MCP client

**Claude Desktop** — add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "k8s": {
      "command": "k8s-mcp-go",
      "args": ["--mode=readonly"]
    }
  }
}
```

**Cursor** — add to `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "k8s": {
      "command": "k8s-mcp-go",
      "args": ["--mode=readonly"]
    }
  }
}
```

> **Note:** If you didn't install to `/usr/local/bin/`, use the full path to the binary in the `command` field.

## Permission Modes

| Mode | Flag | What's included |
|------|------|-----------------|
| 🔵 `readonly` | `--mode=readonly` (default) | 18 read-only tools |
| 🟡 `readwrite` | `--mode=readwrite` | readonly + 6 write tools |
| 🔴 `dangerous` | `--mode=dangerous` | readwrite + 4 destructive tools |

## Tools

### 🔍 Readonly Tools (18)

| Tool | Description |
|------|-------------|
| `list_pods` | List pods in a namespace |
| `get_pod` | Get pod details (containers, status, conditions) |
| `get_pod_logs` | Get pod logs (supports previous instance) |
| `list_services` | List services with type, IP, ports |
| `get_service` | Get service details + endpoints |
| `list_deployments` | List deployments with replica status |
| `get_deployment` | Get deployment details |
| `list_namespaces` | List all namespaces |
| `list_nodes` | List nodes with status, roles, version |
| `cluster_overview` | Cluster health summary with problem detection |
| `get_events` | Get events (filterable by kind/name) |
| `list_configmaps` | List ConfigMaps |
| `get_configmap` | Get ConfigMap data |
| `list_secrets` | List Secrets (keys only) |
| `get_secret` | Get Secret metadata (values hidden by default) |
| `list_pvc` | List PersistentVolumeClaims |
| `list_ingress` | List Ingress resources |
| `list_jobs` | List Jobs |

### ✏️ Readwrite Tools (6)

| Tool | Description |
|------|-------------|
| `restart_deployment` | Rolling restart a deployment |
| `scale_deployment` | Scale deployment replicas |
| `set_image` | Update container image |
| `rollout_status` | Check rollout progress |
| `create_namespace` | Create a new namespace |
| `patch_deployment` | Apply strategic merge patch |

### ⚠️ Dangerous Tools (4)

| Tool | Description |
|------|-------------|
| `delete_pod` | Delete a pod |
| `delete_deployment` | Delete a deployment |
| `delete_namespace` | Delete namespace + ALL resources |
| `apply_yaml` | Apply arbitrary YAML manifest |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `KUBECONFIG` | Path to kubeconfig file (default: `~/.kube/config`) |

## Security Model

- **Default mode is readonly** — AI can only view, never modify
- **Write mode** enables safe mutations (scale, restart, update image)
- **Dangerous mode** required for deletes and arbitrary YAML
- Each tool explicitly checks permissions at runtime
- Permission denied errors include the required mode

## Build from Source

If you want to build locally:

```bash
git clone https://github.com/kaneg/k8s-mcp-go.git
cd k8s-mcp-go
go build -o k8s-mcp-go .
```

## License

MIT
