# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /workspace

# Copy go modules manifests
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY api/ api/
COPY cmd/ cmd/
COPY internal/ internal/

# Fetch latest crawler-user-agents dataset at build time (not vendored in git)
ARG CRAWLER_UA_URL=https://raw.githubusercontent.com/monperrus/crawler-user-agents/master/crawler-user-agents.json
RUN wget -qO internal/server/crawler-user-agents.json "${CRAWLER_UA_URL}"

# Build statically linked and stripped binary
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o operator ./cmd/operator

# Runtime stage: ultra-lean, zero CVEs, pre-packaged CA certificates
FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /workspace/operator /usr/local/bin/operator

USER 65532:65532

ENTRYPOINT ["/usr/local/bin/operator"]
