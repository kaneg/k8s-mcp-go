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

### Option 1: MCPB Install (Recommended)

Download the `.mcpb` bundle for your platform from [Releases](https://github.com/kaneg/k8s-mcp-go/releases/latest). MCPB-compatible clients (Claude Desktop, Cursor, etc.) can install it directly — no Docker, no Go, no manual setup.

| OS | Arch | File |
|----|------|------|
| Linux | x86_64 | `k8s-mcp-go_*_linux_amd64.mcpb` |
| Linux | ARM64 | `k8s-mcp-go_*_linux_arm64.mcpb` |
| macOS | Intel | `k8s-mcp-go_*_darwin_amd64.mcpb` |
| macOS | Apple Silicon | `k8s-mcp-go_*_darwin_arm64.mcpb` |
| Windows | x86_64 | `k8s-mcp-go_*_windows_amd64.mcpb` |
| Windows | ARM64 | `k8s-mcp-go_*_windows_arm64.mcpb` |

### Option 2: Manual Binary Install

Grab the binary archive from [Releases](https://github.com/kaneg/k8s-mcp-go/releases/latest) and extract:

```bash
# Example: Linux x86_64
tar xzf k8s-mcp-go_*_linux_amd64.tar.gz
chmod +x k8s-mcp-go
sudo mv k8s-mcp-go /usr/local/bin/
```

Then add to your MCP client:

**Claude Desktop** (`claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "k8s": {
      "command": "k8s-mcp-go",
      "args": ["-mode=readonly"]
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
      "args": ["-mode=readonly"]
    }
  }
}
```

Restart your client and start asking questions about your cluster.

## Permission Modes

| Mode | Flag | AI can... |
|------|------|-----------|
| 🔵 **readonly** | `-mode=readonly` (default) | View everything, change nothing |
| 🟡 **readwrite** | `-mode=readwrite` | Scale, restart, update images |
| 🔴 **dangerous** | `-mode=dangerous` | Delete resources, apply arbitrary YAML |

Each tool checks permissions at runtime. If AI tries something outside its mode, it gets a clear "permission denied" error — not a silent failure.

## Tools (31 total)

### 🔍 Readonly (20 tools)

| Tool | Description |
|------|-------------|
| `list_pods` | List pods in a namespace |
| `get_pod` | Get pod details |
| `get_pod_logs` | Get pod logs |
| `list_deployments` | List deployments |
| `get_deployment` | Get deployment details |
| `list_statefulsets` | List StatefulSets |
| `get_statefulset` | Get StatefulSet details |
| `list_services` | List services |
| `get_service` | Get service details |
| `list_configmaps` | List ConfigMaps |
| `get_configmap` | Get ConfigMap data |
| `list_secrets` | List Secrets (keys only) |
| `get_secret` | Get Secret metadata and keys |
| `list_pvc` | List PersistentVolumeClaims |
| `list_ingress` | List Ingress resources |
| `list_jobs` | List Jobs |
| `list_nodes` | List cluster nodes |
| `list_namespaces` | List namespaces |
| `cluster_overview` | Cluster health summary |
| `get_events` | List events |

### ✏️ Readwrite (+7 tools)

| Tool | Description |
|------|-------------|
| `scale_deployment` | Scale deployment replicas |
| `restart_deployment` | Rolling restart a deployment |
| `restart_statefulset` | Rolling restart a statefulset |
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
