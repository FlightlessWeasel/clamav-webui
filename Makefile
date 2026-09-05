# ClamAV WebUI

BINARY := clamav-webui
DIST   := dist
VERSION ?= dev

.PHONY: help
help:
	@echo "Targets:"
	@echo "  make web      build the React frontend into internal/webui/dist"
	@echo "  make build    build the frontend, then the Go binary into $(DIST)/"
	@echo "  make run      build web + run the server locally"
	@echo "  make dev      how to run the Go API and the Vite dev server together"
	@echo "  make test     go test ./... and the frontend tests"
	@echo "  make fmt      gofmt -w on all Go sources"
	@echo "  make docker   build the Docker image"

.PHONY: web
web:
	cd web && npm install && npm run build

.PHONY: build
build: web
	mkdir -p $(DIST)
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(DIST)/$(BINARY) ./cmd/clamav-webui

.PHONY: build-go
build-go:
	mkdir -p $(DIST)
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(DIST)/$(BINARY) ./cmd/clamav-webui

.PHONY: run
run: web
	go run ./cmd/clamav-webui

.PHONY: dev
dev:
	@echo "Run these in two terminals:"
	@echo "  CLAMWEB_CONFIG_DIR=./.devstate CLAMWEB_LOG_LEVEL=debug go run ./cmd/clamav-webui"
	@echo "  cd web && npm run dev   # http://localhost:5173"

.PHONY: test
test:
	go test ./...
	@[ -d web/node_modules ] && (cd web && npm test) || echo "skip web tests (run 'npm --prefix web ci' first)"

.PHONY: fmt
fmt:
	gofmt -w ./cmd ./internal

.PHONY: docker
docker:
	docker build --build-arg VERSION=$(VERSION) -t $(BINARY):$(VERSION) .

.PHONY: clean
clean:
	rm -rf $(DIST) internal/webui/dist/assets internal/webui/dist/*.js internal/webui/dist/*.css
