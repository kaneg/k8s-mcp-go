FROM gcr.io/distroless/static-debian12
LABEL io.modelcontextprotocol.server.name="io.github.kaneg/k8s-mcp-go"
COPY k8s-mcp-go /usr/local/bin/k8s-mcp-go
ENTRYPOINT ["k8s-mcp-go"]
