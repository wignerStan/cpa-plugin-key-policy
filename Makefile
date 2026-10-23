PLUGIN := cpa-key-policy
PKG := ./cmd/cpa-key-policy
DIST := dist
WEB := web
EMBED_INDEX := internal/plugin/web/dist/index.html
GOFLAGS ?= -mod=readonly
INFRA_GO_DIR ?= $(HOME)/infra/go

.PHONY: prepare-workspace test test-cpa-materialization web-build build-linux-amd64 build-linux-arm64 build-linux clean

prepare-workspace:
	INFRA_GO_DIR="$(INFRA_GO_DIR)" bash scripts/prepare-go-workspace.sh

test-cpa-materialization:
	python3 scripts/test-cpa-materialization.py

test: prepare-workspace test-cpa-materialization
	GOFLAGS="$(GOFLAGS)" go test ./cmd/... ./internal/...

# Build the single-file web UI and place it where the Go embed expects it.
web-build:
	cd $(WEB) && npm install && VITE_HOSTED=1 npm run build
	cp $(WEB)/dist/index.html $(EMBED_INDEX)

build-linux-amd64: prepare-workspace web-build
	mkdir -p $(DIST)
	GOFLAGS="$(GOFLAGS)" GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -buildvcs=false -tags cshared -buildmode=c-shared -o $(DIST)/$(PLUGIN)_linux_amd64.so $(PKG)

build-linux-arm64: prepare-workspace web-build
	mkdir -p $(DIST)
	GOFLAGS="$(GOFLAGS)" GOOS=linux GOARCH=arm64 CGO_ENABLED=1 go build -buildvcs=false -tags cshared -buildmode=c-shared -o $(DIST)/$(PLUGIN)_linux_arm64.so $(PKG)

build-linux: build-linux-amd64 build-linux-arm64

clean:
	rm -rf $(DIST)
