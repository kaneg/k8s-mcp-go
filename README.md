# k8s-mcp-go

**Safe, read-only-by-default Kubernetes access for AI agents.**

A Kubernetes MCP server that lets AI assistants inspect your cluster safely — without giving them unrestricted `kubectl`.

![GitHub release](https://img.shields.io/github/v/release/kaneg/k8s-mcp-go?sort=semver)
![License](https://img.shields.io/github/license/kaneg/k8s-mcp-go)
![Go](https://img.shields.io/badge/Go-1.24+-00ADD8)
![MCP](https://img.shields.io/badge/MCP-Kubernetes-326CE5)

## What makes it different

- **Read-only by default**: useful for diagnosis without giving AI permission to mutate the cluster.
- **Explicit permission modes**: choose `readonly`, `readwrite`, or `dangerous` before the assistant starts.
- **No Docker required**: install via MCPB bundles or a single native binary.
- **Kubernetes-native visibility**: inspect pods, deployments, services, logs, events, nodes, and live CPU/memory usage.
- **Registry-ready distribution**: published as `io.github.kaneg/k8s-mcp-go` for MCP Registry consumers and packaged clients.

## Why

Most AI agents can use Kubernetes tools.
The real problem is **how to stop them from changing the wrong thing**.

Giving an AI raw `kubectl` access is risky. Even a capable model can:

- delete the wrong resource
- restart the wrong workload
- apply a dangerous change
- turn a debugging session into a production incident

`k8s-mcp-go` gives AI agents a **guardrailed interface** to Kubernetes instead of unrestricted shell access.

It is built for the real-world question:

> How can I let AI help with Kubernetes, without letting it break my cluster?

## Permission Modes

You choose the boundary up front:

| What you want | Mode |
|---------------|------|
| "Let AI inspect and diagnose, but change nothing" | `readonly` |
| "Allow safe operational actions like scale and restart" | `readwrite` |
| "Give it full cluster power" | `dangerous` |

### `readonly` (default)
For diagnosis, inspection, and safe exploration.

AI can do things like:
- list pods, deployments, services, nodes, and namespaces
- read logs and events
- inspect cluster state
- check resource usage

It **cannot** modify workloads or delete resources.

### `readwrite`
For controlled operational workflows.

AI can do things like:
- scale deployments
- restart deployments
- restart statefulsets
- update images
- patch deployments
- create namespaces

It still cannot perform the most destructive operations.

### `dangerous`
Full access.

Use this only when you explicitly want AI to be able to:
- delete resources
- delete namespaces
- apply arbitrary YAML

If you are unsure, use `readonly`.

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

Grab the binary archive from [Releases](https://github.com/kaneg/k8s-mcp-go/releases/latest) and extract it:

```bash
# Example: Linux x86_64
tar xzf k8s-mcp-go_*_linux_amd64.tar.gz
chmod +x k8s-mcp-go
sudo mv k8s-mcp-go /usr/local/bin/
```

Then add it to your MCP client.

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

## Example prompts

Once installed, ask your AI assistant things like:

- "Show me unhealthy pods in all namespaces."
- "Which pods are using the most memory?"
- "Summarize recent warning events."
- "Inspect this deployment and explain why it is not ready."
- "Check rollout status for this deployment."

In `readonly` mode, the assistant can investigate but cannot change the cluster.

## Why not just use kubectl?

Because the problem is not whether AI can talk to Kubernetes.
The problem is whether it can do so **safely**.

`kubectl` is powerful, but it does not give you a product-level permission mode for AI behavior.
With `k8s-mcp-go`, you decide whether the assistant can:

- inspect only
- perform limited operational actions
- or get full control

**The permission boundary is the product.**

## Available Tools (34 total)

Tools are grouped by permission level.

### Readonly (23)

| Tool | Description |
|------|-------------|
| `server_info` | Show server version, mode, kubeconfig path, and runtime details |
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
| `top_nodes` | Current node CPU and memory usage |
| `top_pods` | Current pod CPU and memory usage |
| `list_nodes` | List cluster nodes |
| `list_namespaces` | List namespaces |
| `cluster_overview` | Cluster health summary |
| `get_events` | List events |

### Readwrite (7)

| Tool | Description |
|------|-------------|
| `scale_deployment` | Scale deployment replicas |
| `restart_deployment` | Rolling restart a deployment |
| `restart_statefulset` | Rolling restart a statefulset |
| `set_image` | Update container image |
| `rollout_status` | Check rollout progress |
| `create_namespace` | Create a new namespace |
| `patch_deployment` | Apply strategic merge patch |

### Dangerous (4)

| Tool | Description |
|------|-------------|
| `delete_pod` | Delete a pod |
| `delete_deployment` | Delete a deployment |
| `delete_namespace` | Delete a namespace and all resources |
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
