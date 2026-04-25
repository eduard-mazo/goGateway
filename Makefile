.PHONY: build frontend embed backend backend-win build-win clean run

BIN := goGateway
FRONT_DIST := frontend/dist
EMBED_DIST := backend/internal/web/dist

# HTTP listen address for `make run`. Override: `make run PORT=9090` or `make run HTTP=0.0.0.0:9090`.
PORT ?= 8080
HTTP ?= :$(PORT)

# IEC 104 frame-trace logging. `make build DEBUG=1` bakes "on" as the binary
# default; `GW_IEC_DEBUG=1` at runtime still overrides either way.
DEBUG ?= 0
LDFLAGS := -X goGateway/internal/config.DefaultHTTPListen=$(HTTP) -X goGateway/internal/iec104.DefaultDebug=$(DEBUG)

build: frontend embed backend

frontend:
	pnpm --dir frontend install --frozen-lockfile || pnpm --dir frontend install
	pnpm --dir frontend build

embed: frontend
	rm -rf $(EMBED_DIST)
	cp -r $(FRONT_DIST) $(EMBED_DIST)

backend:
	@echo "  HTTP=$(HTTP)  DEBUG=$(DEBUG)"
	cd backend && go build -ldflags "$(LDFLAGS)" -o ../$(BIN) ./cmd/gateway

build-win: frontend embed backend-win

backend-win:
	@echo "  HTTP=$(HTTP)  DEBUG=$(DEBUG)  (windows/amd64)"
	cd backend && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o ../$(BIN).exe ./cmd/gateway

run: build
	GW_HTTP=$(HTTP) GW_IEC_DEBUG=$(DEBUG) ./$(BIN)

clean:
	rm -f $(BIN) $(BIN).exe
	rm -rf $(EMBED_DIST) $(FRONT_DIST)
