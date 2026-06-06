# Agent Guidelines

## Avoid Local-Only Fixes

Before changing a local behavior, first decide whether it represents a broader pattern.

For API, schema, MCP tool, permission, or output-format changes:

1. Search for peer implementations.
2. Identify the shared contract.
3. Propose the consistent rule.
4. Apply the rule to the representative set, not only the first symptom.
5. If intentionally scoped narrowly, state why.

In this repo, MCP tool inputs and outputs should be treated as a tool-surface contract. Keep namespaced resource behavior, output formats, permission exposure, and truncation/full-detail controls consistent across similar tools.

## Design Tools For AI Use

MCP tools should be easy for an AI agent to choose and use correctly.

For every tool:

1. Make the description explicit about scope, defaults, and important options.
2. Prefer schema-visible parameters over hidden string conventions.
3. Return output that helps the next tool choice, such as namespace, kind, controller/owner, selectors, and related resource names.
4. Use concise structured text or explicit `json`/`yaml` modes instead of vague prose.
5. When a tool returns an error, include actionable context when possible, not only the raw Kubernetes error.

If an agent has to guess the resource kind, namespace scope, or next lookup from a tool result, improve the tool contract.
