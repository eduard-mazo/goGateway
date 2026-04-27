.PHONY: build frontend embed backend backend-win backend-rh-amd64 backend-rh-ppc64le build-win build-rh-amd64 build-rh-ppc64le run run-win test vet clean help

BIN        := goGateway
FRONT_DIST := frontend/dist
EMBED_DIST := backend/internal/web/dist

# HTTP listen address. Override: make run PORT=9090  or  make run HTTP=0.0.0.0:9090
PORT ?= 8080
HTTP ?= :$(PORT)

# IEC 104 frame-trace logging.
# make build DEBUG=1  bakes "on" as binary default.
# GW_IEC_DEBUG=1 at runtime overrides either way.
DEBUG ?= 0
LDFLAGS := -X goGateway/internal/config.DefaultHTTPListen=$(HTTP) \
           -X goGateway/internal/iec104.DefaultDebug=$(DEBUG)

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

##@ Misc

clean:  ## Remove build artefacts (binaries + dist directories)
	rm -f $(BIN) $(BIN).exe $(BIN)-linux-amd64 $(BIN)-linux-ppc64le
	rm -rf $(EMBED_DIST) $(FRONT_DIST)

help:  ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} \
	  /^[a-zA-Z_-]+:.*##/ { printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2 } \
	  /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
