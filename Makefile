.PHONY: help dev run build bin css css-watch test tidy
.DEFAULT_GOAL := help

TAILWIND_BIN ?= tailwindcss

# ponytail: Go 1.22.4 on macOS 26 (Darwin 25) needs two fixups to produce a
# runnable binary, so we build + sign in one step (see the `sign` macro below):
#   1. the internal linker omits LC_UUID, which dyld rejects -> use the system linker
#   2. the external linker leaves an invalid ad-hoc signature, which Apple Silicon
#      SIGKILLs at exec (looks like "connection refused") -> re-sign ad-hoc
# Drop CGO/LDFLAGS/sign once on a Go that links a valid LC_UUID internally (>= 1.23).
export CGO_ENABLED = 1
LDFLAGS = -ldflags=-linkmode=external
sign = codesign -f -s -

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

dev: ## Live-reload dev server (air)
	air

run: css bin ## Build, sign, run, and open the browser
	./tmp/main --open

build: css ## Build signed binary to ./claudarium
	go build $(LDFLAGS) -o claudarium ./cmd/app
	$(sign) claudarium

bin: ## Build signed dev binary to ./tmp/main (used by air)
	go build $(LDFLAGS) -o ./tmp/main ./cmd/app
	$(sign) ./tmp/main

css: ## Build Tailwind CSS
	$(TAILWIND_BIN) -i ./web/static/css/input.css -o ./web/static/css/app.css --minify

css-watch: ## Watch and rebuild CSS
	$(TAILWIND_BIN) -i ./web/static/css/input.css -o ./web/static/css/app.css --watch

test: ## Run tests
	go test $(LDFLAGS) ./...

tidy: ## Sync go modules
	go mod tidy
