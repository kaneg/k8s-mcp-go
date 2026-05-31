# k8s-mcp-go

A high-performance Kubernetes MCP Server written in Go. Bridges AI agents (Claude, Cursor, Codex, etc.) to your K8s cluster.

## Features

- **25 MCP Tools** — read, write, and dangerous operations
- **3-tier permission model** — readonly (default), readwrite, dangerous
- **Zero configuration** — auto-detects `~/.kube/config`
- **Single binary** — no dependencies, no runtime
- **Fast** — Go implementation, instant startup

## Permission Modes

| Mode | Flag | Tools Available |
|------|------|-----------------|
| `readonly` | `--mode=readonly` (default) | 18 read-only tools |
| `readwrite` | `--mode=readwrite` | + 6 write tools |
| `dangerous` | `--mode=dangerous` | + 4 destructive tools |

## Quick Start

```bash
# Build
go build -o k8s-mcp-go .

# Run in readonly mode (default, safest)
./k8s-mcp-go

# Run with write access
./k8s-mcp-go --mode=readwrite

# Run with full access (careful!)
./k8s-mcp-go --mode=dangerous
```

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

## MCP Client Configuration

### Claude Desktop

```json
{
  "mcpServers": {
    "k8s": {
      "command": "/path/to/k8s-mcp-go",
      "args": ["--mode=readonly"]
    }
  }
}
```

### Cursor

Add to `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "k8s": {
      "command": "/path/to/k8s-mcp-go",
      "args": ["--mode=readwrite"]
    }
  }
}
```

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

## License

MIT
