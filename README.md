# k8s-mcp-go

A high-performance Kubernetes MCP Server written in Go.

Single binary, zero dependencies beyond kubectl config — connects AI agents to your K8s cluster via the [Model Context Protocol](https://modelcontextprotocol.io).

## Why?

Existing K8s MCP servers are written in Python/TypeScript. This one is:

- **Fast** — Go native, starts in milliseconds
- **Lightweight** — single binary, ~15MB
- **Zero config** — uses your existing `~/.kube/config`
- **Read-only (MVP)** — safe to run, no mutations

## Tools

| Tool | Description |
|------|-------------|
| `list_pods` | List pods in a namespace |
| `get_pod` | Get detailed pod info (containers, status, events) |
| `get_pod_logs` | Get pod logs (with tail, container selection) |
| `list_services` | List services in a namespace |
| `get_service` | Get detailed service info + endpoints |
| `list_deployments` | List deployments in a namespace |
| `get_deployment` | Get deployment status & conditions |
| `list_namespaces` | List all namespaces |
| `list_nodes` | List nodes with resource info |
| `cluster_overview` | Cluster-wide summary (health, counts, issues) |

## Install

```bash
go install github.com/kaneg/k8s-mcp-go@latest
```

Or download from [Releases](https://github.com/kaneg/k8s-mcp-go/releases).

## Usage

```bash
# Run with default kubeconfig (~/.kube/config)
k8s-mcp-go

# Use custom kubeconfig
KUBECONFIG=/path/to/config k8s-mcp-go
```

### With Claude Desktop / Cursor / Codex

```json
{
  "mcpServers": {
    "k8s": {
      "command": "k8s-mcp-go"
    }
  }
}
```

## Example Questions

- "What pods are running in the default namespace?"
- "Why is my pod CrashLoopBackOff?"
- "Show me the cluster overview"
- "Which nodes have high CPU usage?"
- "What services are exposed in the kube-system namespace?"

## License

MIT
