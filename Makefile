.PHONY: build frontend embed backend backend-win backend-rh-amd64 backend-rh-ppc64le build-win build-rh-amd64 build-rh-ppc64le run run-win test vet clean help image-ppc64le export-ppc64le dev-container

BIN        := goGateway
FRONT_DIST := frontend/dist
EMBED_DIST := backend/internal/web/dist

# Container image artefacts
IMAGE_NAME := gogateway:ppc64le          # production — loaded on target via docker load
IMAGE_DEV  := gogateway:dev              # dev/test — runs natively on this host
IMAGE_TAR  := goGateway-ppc64le.tar

# HTTP listen address. Override: make run PORT=9090  or  make run HTTP=0.0.0.0:9090
PORT ?= 8080
HTTP ?= :$(PORT)

# IEC 104 frame-trace logging.
# make build DEBUG=1  bakes "on" as binary default.
# GW_IEC_DEBUG=1 at runtime overrides either way.
DEBUG ?= 0
LDFLAGS := -X goGateway/internal/config.DefaultHTTPListen=$(HTTP) \
           -X goGateway/internal/iec104.DefaultDebug=$(DEBUG)

# Container builds strip debug info and symbol tables (-s -w) for a smaller
# binary.  -trimpath removes local build-host paths from the binary.
LDFLAGS_CONTAINER := $(LDFLAGS) -s -w

##@ Build

build: frontend embed backend  ## Full Linux build (frontend + embed + Go binary)

build-win: frontend embed backend-win  ## Full Windows/amd64 cross-build

build-rh-amd64: frontend embed backend-rh-amd64  ## Full Red Hat/x86_64 cross-build

build-rh-ppc64le: frontend embed backend-rh-ppc64le  ## Full Red Hat/ppc64le cross-build

frontend:  ## Build the Vue 3 frontend (pnpm install + vite build)
	pnpm --dir frontend install --frozen-lockfile || pnpm --dir frontend install
	pnpm --dir frontend build

embed: frontend  ## Copy frontend dist into the Go embed tree
	rm -rf $(EMBED_DIST)
	cp -r $(FRONT_DIST) $(EMBED_DIST)

backend:  ## Compile Go binary for the current OS/arch
	@echo "  HTTP=$(HTTP)  DEBUG=$(DEBUG)"
	cd backend && go build -ldflags "$(LDFLAGS)" -o ../$(BIN) ./cmd/gateway

backend-win:  ## Cross-compile Go binary for Windows/amd64 (CGO_ENABLED=0)
	@echo "  HTTP=$(HTTP)  DEBUG=$(DEBUG)  (windows/amd64)"
	cd backend && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
	  go build -ldflags "$(LDFLAGS)" -o ../$(BIN).exe ./cmd/gateway

backend-rh-amd64:  ## Cross-compile Go binary for Red Hat Linux/x86_64 (CGO_ENABLED=0)
	@echo "  HTTP=$(HTTP)  DEBUG=$(DEBUG)  (linux/amd64 rhel)"
	cd backend && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
	  go build -ldflags "$(LDFLAGS)" -o ../$(BIN)-linux-amd64 ./cmd/gateway

backend-rh-ppc64le:  ## Cross-compile Go binary for Red Hat Linux/ppc64le (CGO_ENABLED=0)
	@echo "  HTTP=$(HTTP)  DEBUG=$(DEBUG)  (linux/ppc64le rhel)"
	cd backend && GOOS=linux GOARCH=ppc64le CGO_ENABLED=0 \
	  go build -ldflags "$(LDFLAGS)" -o ../$(BIN)-linux-ppc64le ./cmd/gateway

##@ Run

run: build  ## Build then run (Linux). PORT= or HTTP= to override listen address.
	GW_HTTP=$(HTTP) GW_IEC_DEBUG=$(DEBUG) ./$(BIN)

run-win: build-win  ## Build Windows binary then run it via Wine (requires wine)
	GW_HTTP=$(HTTP) GW_IEC_DEBUG=$(DEBUG) wine $(BIN).exe

##@ Quality

test:  ## Run Go unit tests
	cd backend && go test ./...

vet:  ## Run go vet on all backend packages
	cd backend && go vet ./...

##@ Container (ppc64le offline deployment)

image-ppc64le: frontend embed  ## Build stripped ppc64le static binary + OCI image (FROM scratch)
	@echo "  Building stripped ppc64le binary..."
	cd backend && GOOS=linux GOARCH=ppc64le CGO_ENABLED=0 \
	  go build -trimpath -ldflags "$(LDFLAGS_CONTAINER)" \
	  -o ../$(BIN)-linux-ppc64le ./cmd/gateway
	@echo "  Building OCI image $(IMAGE_NAME)..."
	docker build \
	  --platform linux/ppc64le \
	  --file deploy/Dockerfile.ppc64le \
	  --tag $(IMAGE_NAME) \
	  --no-cache \
	  .

export-ppc64le: image-ppc64le  ## Export ppc64le OCI image to tar for offline transfer
	docker save --output $(IMAGE_TAR) $(IMAGE_NAME)
	@echo ""
	@echo "  Artefact: $(IMAGE_TAR)  ($$(du -sh $(IMAGE_TAR) | cut -f1))"
	@echo "  Load on target:  podman load -i $(IMAGE_TAR)"

dev-container: frontend embed  ## Build + run dev container on host OS (linux/amd64) — Ctrl-C to stop
	@echo "  Building linux/amd64 binary for dev container..."
	cd backend && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
	  go build -trimpath -ldflags "$(LDFLAGS_CONTAINER)" \
	  -o ../$(BIN)-linux-amd64 ./cmd/gateway
	docker build \
	  --platform linux/amd64 \
	  --file deploy/Dockerfile.dev \
	  --tag $(IMAGE_DEV) \
	  .
	@mkdir -p dev-data
	docker run --rm -it \
	  --name goGateway-dev \
	  --network host \
	  -v $(CURDIR)/dev-data:/data \
	  -e GW_DB=/data/gateway.db \
	  -e GW_HTTP=$(HTTP) \
	  -e GW_IEC_DEBUG=1 \
	  $(IMAGE_DEV)

##@ Misc

clean:  ## Remove build artefacts (binaries + dist directories)
	rm -f $(BIN) $(BIN).exe $(BIN)-linux-amd64 $(BIN)-linux-ppc64le $(IMAGE_TAR)
	rm -rf $(EMBED_DIST) $(FRONT_DIST)

help:  ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} \
	  /^[a-zA-Z_-]+:.*##/ { printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2 } \
	  /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
