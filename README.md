# k8s-mcp-go

**Safe Kubernetes access for AI agents.** 

You want AI to *look at* your cluster. You don't want it to *accidentally delete everything in production*. This MCP server lets you draw the line.

```bash
# AI can only read. No writes. No surprises.
k8s-mcp-go
```

## Why

Giving AI agents raw `kubectl` access is terrifying. There's nothing stopping it from running `kubectl delete ns production` when it's confused. And it *will* get confused eventually.

k8s-mcp-go sits between your AI and the cluster with a simple rule: **you decide what it can do**.

| What you want | Mode |
|---------------|------|
| "Let AI explore and diagnose, but don't touch anything" | `--mode=readonly` (default) |
| "OK, it can scale and restart deployments" | `--mode=readwrite` |
| "Full access, I know what I'm doing" | `--mode=dangerous` |

That's it. Default is readonly. Everything else is opt-in.

## Quick Start

### 1. Download

Grab the latest binary from [Releases](https://github.com/kaneg/k8s-mcp-go/releases/latest).

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

### 2. Add to your MCP client

**Claude Desktop** (`claude_desktop_config.json`):

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

**Cursor** (`.cursor/mcp.json`):

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

That's it. Restart your client and start asking questions about your cluster.

## Permission Modes

| Mode | Flag | AI can... |
|------|------|-----------|
| 🔵 **readonly** | `--mode=readonly` (default) | View everything, change nothing |
| 🟡 **readwrite** | `--mode=readwrite` | Scale, restart, update images |
| 🔴 **dangerous** | `--mode=dangerous` | Delete resources, apply arbitrary YAML |

Each tool checks permissions at runtime. If AI tries something outside its mode, it gets a clear "permission denied" error — not a silent failure.

## Tools

### 🔍 Readonly (18 tools)

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

### ✏️ Readwrite (+7 tools)

| Tool | Description |
|------|-------------|
| `restart_deployment` | Rolling restart a deployment |
| `restart_statefulset` | Rolling restart a statefulset |
| `scale_deployment` | Scale deployment replicas |
| `set_image` | Update container image |
| `rollout_status` | Check rollout progress |
| `create_namespace` | Create a new namespace |
| `patch_deployment` | Apply strategic merge patch |

### ⚠️ Dangerous (+4 tools)

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

## Build from Source

```bash
git clone https://github.com/kaneg/k8s-mcp-go.git
cd k8s-mcp-go
go build -o k8s-mcp-go .
```

## License

MIT
