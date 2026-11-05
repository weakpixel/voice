# Every Go module of the repository; each is formatted, vetted and tested on
# its own. A nested sf2 module joins the list when it is added.
MODULES = . ebiten editor

# Stamped into the editor's index.html so browsers fetch a new build.
VERSION ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)

# js/wasm. WASM_ENV runs go test under go_js_wasm_exec (Node): it puts the
# exec wrapper on PATH and starts from an empty environment, because Node
# refuses a command line plus environment over its size limit, which a large
# CI or container environment exceeds.
WASM_ENV = env -i HOME="$(HOME)" PATH="$(shell go env GOROOT)/lib/wasm:$(PATH)" \
	GOPATH="$(shell go env GOPATH)" GOMODCACHE="$(shell go env GOMODCACHE)" \
	GOCACHE="$(shell go env GOCACHE)" GOTOOLCHAIN="$(shell go env GOTOOLCHAIN)" \
	GOOS=js GOARCH=wasm

.PHONY: all
all:
	@for m in $(MODULES); do \
		echo "== $$m"; \
		(cd $$m && go fmt ./... && go vet ./... && go test -race ./...) || exit 1; \
	done

# Core tests under js/wasm (needs Node).
.PHONY: wasm-test
wasm-test:
	@command -v node >/dev/null || { echo "wasm-test: node not found"; exit 1; }
	$(WASM_ENV) go vet ./...
	$(WASM_ENV) go test ./...
	cd ebiten && $(WASM_ENV) go vet ./...
	cd editor && $(WASM_ENV) go vet ./...

# Benchmarks of the synthesizer (and later the Speaker), native.
.PHONY: bench
bench:
	go test -run '^$$' -bench . -benchmem ./...

# The same benchmarks under js/wasm (Node).
.PHONY: bench-wasm
bench-wasm:
	$(WASM_ENV) go test -run '^$$' -bench . -benchmem ./...

# Renders every built-in test patch to build/patches.wav.
.PHONY: patches
patches:
	go run ./cmd/voiceplay -patches -o build/patches.wav

# The Voice Editor's static site in build/editor (what GitHub Pages serves).
.PHONY: editor
editor:
	rm -rf build/editor
	cd editor && go run ./cmd/mksite -out ../build/editor -version $(VERSION)
	cd editor && GOOS=js GOARCH=wasm go build -trimpath -ldflags=-s -o ../build/editor/voice.wasm ./wasm

# Unit tests of the editor page's scripts (Node, no packages).
.PHONY: editor-test
editor-test:
	node --test editor/web/test/*.test.mjs

# Serves build/editor on http://localhost:8000.
.PHONY: editor-serve
editor-serve: editor
	node editor/smoke/serve.cjs build/editor 8000

# Headless Chromium test of the built site (needs Playwright with Chromium,
# found through NODE_PATH or installed globally).
.PHONY: editor-smoke
editor-smoke: editor
	node editor/smoke/run.cjs build/editor
