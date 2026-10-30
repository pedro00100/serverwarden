# The final image holds only the panel binary and the agents it offers to
# servers: no Node, no Go, no SSH client, no source code.
#
# Multi-platform builds (linux/amd64, linux/arm64, linux/arm/v7) run Node and
# Go on the build machine and cross-compile; only the last stage is per target.

FROM --platform=$BUILDPLATFORM docker.io/library/node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.27-alpine AS go
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-buildvcs=false
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY cmd ./cmd
COPY internal ./internal
COPY examples ./examples
RUN rm -rf internal/adapters/web/static
COPY --from=web /web/build ./internal/adapters/web/static
ARG VERSION=dev
ENV LDFLAGS="-s -w -X github.com/pedro00100/serverwarden/internal/brand.Version=${VERSION}"
# Tests and the agents do not depend on the target: built once for every platform.
RUN go vet ./... && go test ./... \
 && for arch in amd64 arm64 arm; do \
      GOOS=linux GOARCH=$arch GOARM=7 go build -trimpath -ldflags="$LDFLAGS" -o /out/agents/serverwarden-agent-linux-$arch ./cmd/agent; \
    done
ARG TARGETOS TARGETARCH TARGETVARIANT
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
      go build -trimpath -ldflags="$LDFLAGS" -o /out/serverwarden ./cmd/panel

FROM docker.io/library/alpine:3.24
# The data directory exists and belongs to the panel user, so a named volume
# mounted there needs no chown.
RUN adduser -D -H -u 10001 -s /sbin/nologin panel \
 && install -d -o 10001 -g 10001 -m 0700 /var/lib/serverwarden
COPY --from=go /out/serverwarden /usr/local/bin/serverwarden
# Offered to servers by the install script (one per CPU); they never run here.
COPY --from=go /out/agents /opt/serverwarden-agent
USER 10001:10001
ENV HOME=/tmp
VOLUME /var/lib/serverwarden
EXPOSE 8443
HEALTHCHECK --interval=30s --timeout=6s --start-period=15s --retries=3 \
  CMD ["/usr/local/bin/serverwarden", "-health", "https://127.0.0.1:8443/api/health"]
ENTRYPOINT ["/usr/local/bin/serverwarden"]
