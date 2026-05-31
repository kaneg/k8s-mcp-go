# Build
FROM golang:1.22-bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o k8s-mcp-go .

# Runtime
FROM gcr.io/distroless/static-debian12
LABEL io.modelcontextprotocol.server.name="io.github.kaneg/k8s-mcp-go"
COPY --from=builder /app/k8s-mcp-go /usr/local/bin/k8s-mcp-go
ENTRYPOINT ["k8s-mcp-go"]
