# Development without installing Go or Node: everything runs in throwaway
# containers (Podman or Docker). Caches live in volumes prefixed "panel-" so
# `make clean-all` removes every trace.

ENGINE    ?= $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)
IMG_GO    ?= docker.io/library/golang:1.26-alpine
IMG_NODE  ?= docker.io/library/node:22-alpine
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

GO   = $(ENGINE) run --rm -v "$(CURDIR)":/src:Z -w /src \
       -v panel-go-mod:/go/pkg/mod -v panel-go-cache:/root/.cache/go-build \
       -e CGO_ENABLED=0 -e GOFLAGS=-buildvcs=false $(IMG_GO)
NODE = $(ENGINE) run --rm -v "$(CURDIR)/web":/web:Z -w /web -v panel-npm:/root/.npm $(IMG_NODE)

.PHONY: help test test-integration vet fmt tidy web dev build smoke ui-check install-e2e screenshots image clean clean-all

help: ## show this help
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

test: ## run the Go tests
	$(GO) go test ./...

test-integration: ## downloads the real Trivy and scans packages and an image (slow)
	$(GO) go test -tags integration -run TestRealScan -v -timeout 30m ./internal/adapters/trivy/

vet: ## static analysis of the Go code
	$(GO) go vet ./...

fmt: ## format the Go code
	$(GO) gofmt -w cmd internal

tidy: ## update go.mod and go.sum
	$(GO) go mod tidy

web: ## build the UI into web/build
	$(NODE) sh -c 'npm ci --no-audit --no-fund && npm run build'

dev: ## UI with sample data at http://localhost:5173 (MOCK_LANG=es for Spanish)
	$(ENGINE) run --rm -it -p 127.0.0.1:5173:5173 -e MOCK=1 -e MOCK_LANG=$(MOCK_LANG) -e MOCK_SCREEN=$(MOCK_SCREEN) -v "$(CURDIR)/web":/web:Z -w /web -v panel-npm:/root/.npm $(IMG_NODE) \
		sh -c 'npm ci --no-audit --no-fund && npx vite dev --host 0.0.0.0 --port 5173'

ui-check: ## with `make dev` running: open every page in headless Chromium, fail on JS errors (BASE=url for another server)
	mkdir -p test/shots
	$(ENGINE) run --rm --network host -e BASE=$(BASE) -v "$(CURDIR)/test":/test:Z -v "$(CURDIR)/test/shots":/shots:Z docker.io/library/alpine:3.22 sh /test/ui-check.sh

build: ## panel for this machine, agents for amd64, arm64 and armv7 in out/
	$(GO) sh -c 'go build -trimpath -ldflags="-s -w" -o out/serverwarden ./cmd/panel && \
	             for a in amd64 arm64 arm; do GOOS=linux GOARCH=$$a GOARM=7 go build -trimpath -ldflags="-s -w" -o out/agents/serverwarden-agent-linux-$$a ./cmd/agent; done'

smoke: build ## end-to-end test of the panel binary in a throwaway container
	$(ENGINE) run --rm -v "$(CURDIR)/out":/out:ro,Z -v "$(CURDIR)/test/smoke.sh":/smoke.sh:ro,Z docker.io/library/alpine:3.22 sh /smoke.sh

screenshots: ## with `make dev` running (English sample data): the README images in docs/screenshots
	mkdir -p docs/screenshots
	$(ENGINE) run --rm --network host -e BASE=$(BASE) -v "$(CURDIR)/test":/test:Z -v "$(CURDIR)/docs/screenshots":/shots:Z docker.io/library/alpine:3.22 sh /test/ui-check.sh screenshots.mjs

install-e2e: image ## the real image, a Debian 13 server and Chromium: add the server from the UI and run its install script
	ENGINE=$(ENGINE) sh test/install-e2e.sh serverwarden:$(VERSION)

image: ## container image serverwarden:VERSION
	$(ENGINE) build $(if $(findstring podman,$(ENGINE)),--format docker) --build-arg VERSION=$(VERSION) -t serverwarden:$(VERSION) .

clean: ## remove build output inside the project
	rm -rf out web/build web/.svelte-kit web/node_modules

clean-all: clean ## also remove caches and the project's images
	-$(ENGINE) volume rm panel-go-mod panel-go-cache panel-npm
	-$(ENGINE) image rm $(IMG_GO) $(IMG_NODE)
	-$(ENGINE) images --format '{{.Repository}}:{{.Tag}}' | grep '/serverwarden:' | xargs -r $(ENGINE) image rm
