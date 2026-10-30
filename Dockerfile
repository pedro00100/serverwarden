# The final image holds only the panel binary and the agents it offers to
# servers: no Node, no Go, no SSH client, no source code.

FROM docker.io/library/node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM docker.io/library/golang:1.26-alpine AS go
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-buildvcs=false
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY cmd ./cmd
COPY internal ./internal
COPY examples ./examples
RUN rm -rf internal/adapters/web/static
COPY --from=web /web/build ./internal/adapters/web/static
RUN go vet ./... && go test ./... \
 && go build -trimpath -ldflags="-s -w" -o /out/serverwarden ./cmd/panel \
 && for arch in amd64 arm64 arm; do \
      GOOS=linux GOARCH=$arch GOARM=7 go build -trimpath -ldflags="-s -w" -o /out/agents/serverwarden-agent-linux-$arch ./cmd/agent; \
    done

FROM docker.io/library/alpine:3.22
RUN adduser -D -H -u 10001 -s /sbin/nologin panel
COPY --from=go /out/serverwarden /usr/local/bin/serverwarden
# Offered to servers by the install script (one per CPU); they never run here.
COPY --from=go /out/agents /opt/serverwarden-agent
USER 10001:10001
ENV HOME=/tmp
EXPOSE 8443
HEALTHCHECK --interval=30s --timeout=6s --start-period=15s --retries=3 \
  CMD ["/usr/local/bin/serverwarden", "-health", "https://127.0.0.1:8443/api/health"]
ENTRYPOINT ["/usr/local/bin/serverwarden"]
