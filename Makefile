# Commands used often enough to be worth not retyping. Everything here is a
# thin wrapper: the binary and `go` remain the real interface, and no target
# hides a step you would need to know about when something goes wrong.

BINARY      := personal-web
BUILD_DIR   := bin
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# The front-end toolchain. Both are pinned: templ generates Go that is
# committed, and tailwind generates CSS that is embedded, so the version that
# produced them is part of what the build means (ADR-0013). Neither is needed
# to build, only to change what they generate.
TEMPL_VERSION    := v0.3.1020
TAILWIND_VERSION := v4.3.3
TEMPL            := $(BUILD_DIR)/templ
TAILWIND         := $(BUILD_DIR)/tailwindcss

# The asset name of the standalone tailwind binary for this machine. Written
# with make's own functions rather than a shell case: a `)` inside $(shell ...)
# closes the expression early, and the error it produces names nothing useful.
UNAME_S           := $(shell uname -s)
UNAME_M           := $(shell uname -m)
TAILWIND_OS       := $(if $(filter Darwin,$(UNAME_S)),macos,linux)
TAILWIND_ARCH     := $(if $(filter arm64 aarch64,$(UNAME_M)),arm64,x64)
TAILWIND_PLATFORM := $(TAILWIND_OS)-$(TAILWIND_ARCH)

# The port the server listens on, and the port `make watch` puts the reloading
# browser proxy in front of it on.
PORT       ?= 8080
PROXY_PORT ?= 7331

# The local development database. Both values match the defaults compiled into
# the binary (config.DevDatabaseURL) and the fallbacks in database.yml, so a
# fresh clone needs no environment at all.
DB_CONTAINER := pw-postgres
DB_IMAGE     := postgres:17-alpine
DB_PORT      ?= 5432
DB_NAME      ?= pw_dev

.DEFAULT_GOAL := help

## help: list the available targets
help:
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | awk -F': ' '{printf "  \033[1m%-16s\033[0m %s\n", $$1, $$2}'

# --- building and running ---------------------------------------------------

## build: compile the binary into bin/, stamping the version
#
# This needs no tools: the generated Go and the generated CSS are committed, so
# a fresh clone builds with `go build` alone (ADR-0013). Run `make generate`
# after changing a .templ file or a class name.
build:
	@mkdir -p $(BUILD_DIR)
	go build -ldflags "-X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY) .

## run: start the HTTP server (migrate first; serve refuses a stale schema)
run:
	go run . serve

## config: print the effective configuration, passwords redacted
config:
	go run . config show

# --- front end --------------------------------------------------------------

## generate: rebuild the templ components and the stylesheet
generate: templ css

## templ: compile internal/view/*.templ into Go
templ: $(TEMPL)
	@$(TEMPL) generate --log-level error

## css: rebuild the embedded stylesheet from the templ sources
#
# Tailwind reads those files as plain text, so only class names written out in
# full are found. A class assembled in Go is never generated.
css: $(TAILWIND)
	@$(TAILWIND) --input internal/view/app.src.css \
		--output internal/admin/static/app.css --minify

## watch: rebuild and reload the browser as you edit
#
# Three watchers, each owning one leg of the loop: tailwind regenerates the
# stylesheet, templ regenerates the components and proxies the browser on
# PROXY_PORT, and air (.air.toml) rebuilds and restarts the server whenever the
# generated Go or the generated CSS changes.
#
# Open http://localhost:$(PROXY_PORT), not $(PORT): only the proxy injects the
# reload script.
watch: $(TEMPL) $(TAILWIND)
	@trap 'kill 0' EXIT INT TERM; \
	$(TAILWIND) --input internal/view/app.src.css \
		--output internal/admin/static/app.css --watch & \
	$(TEMPL) generate --watch \
		--proxy=http://localhost:$(PORT) --proxyport=$(PROXY_PORT) & \
	go tool air

# Both tools provide themselves, the way db-up provides its own postgres. They
# land in bin/, which is already ignored.
$(TEMPL):
	@mkdir -p $(BUILD_DIR)
	GOBIN=$(CURDIR)/$(BUILD_DIR) go install github.com/a-h/templ/cmd/templ@$(TEMPL_VERSION)

$(TAILWIND):
	@mkdir -p $(BUILD_DIR)
	@echo "fetching tailwindcss $(TAILWIND_VERSION)"
	@curl -sfL -o $(TAILWIND) \
		https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-$(TAILWIND_PLATFORM)
	@chmod +x $(TAILWIND)

# --- admin ------------------------------------------------------------------

## hash-password: print an argon2id hash for PW_ADMIN_PASSWORD_HASH
hash-password:
	@go run . admin hash-password

## session-secret: print a random value for PW_SESSION_SECRET
session-secret:
	@head -c 32 /dev/urandom | base64

# --- database ---------------------------------------------------------------

## db-up: start a local postgres in docker and wait for it
#
# DB_PORT defaults to 5432 because that is the port compiled into
# config.DevDatabaseURL. If something else already holds it — very likely your
# own postgres — either use that one and skip this target, or move ours and say
# so, since the binary will still be looking at 5432:
#
#   make db-up DB_PORT=5433
#   export PW_DATABASE_URL=postgres://postgres:postgres@127.0.0.1:5433/pw_dev?sslmode=disable
db-up:
	@docker start $(DB_CONTAINER) >/dev/null 2>&1 || \
		docker run -d --name $(DB_CONTAINER) \
			-e POSTGRES_PASSWORD=postgres \
			-e POSTGRES_DB=$(DB_NAME) \
			-p $(DB_PORT):5432 $(DB_IMAGE) >/dev/null 2>&1 || { \
			docker rm -f $(DB_CONTAINER) >/dev/null 2>&1; \
			echo "could not bind port $(DB_PORT); something already listens there."; \
			echo "use that database, or: make db-up DB_PORT=5433 (and set PW_DATABASE_URL to match)"; \
			exit 1; \
		}
	@until docker exec $(DB_CONTAINER) pg_isready -q -U postgres 2>/dev/null; do sleep 1; done
	@echo "postgres ready on :$(DB_PORT)"

## db-down: stop the local postgres, keeping its data
db-down:
	@docker stop $(DB_CONTAINER) >/dev/null 2>&1 || true

## db-reset: destroy the local postgres and its data, then start a fresh one
db-reset:
	@docker rm -f $(DB_CONTAINER) >/dev/null 2>&1 || true
	@$(MAKE) --no-print-directory db-up

## psql: open a psql shell on the local database
psql:
	@docker exec -it $(DB_CONTAINER) psql -U postgres -d $(DB_NAME)

# --- migrations -------------------------------------------------------------

## migrate: apply every pending migration
migrate:
	go run . migrate up

## migrate-down: roll back migrations (make migrate-down step=2)
migrate-down:
	go run . migrate down --step $(or $(step),1)

## migrate-status: show which migrations have been applied
migrate-status:
	go run . migrate status

## migration: scaffold an up/down pair (make migration name=add_tags)
#
# This is the one thing soda would have done for us. Generating the pair here
# keeps the timestamp format right without needing soda installed.
migration:
ifndef name
	$(error usage: make migration name=add_tags)
endif
	@ts=$$(date -u +%Y%m%d%H%M%S); \
	dir=internal/database/migrations; \
	for dir_file in $$dir/$${ts}_$(name).up.sql $$dir/$${ts}_$(name).down.sql; do \
		printf -- "-- %s\n" "$$(basename $$dir_file)" > $$dir_file; \
		echo "created $$dir_file"; \
	done

# --- checks -----------------------------------------------------------------

## test: run every test (content tests start postgres via testcontainers)
test:
	go test ./...

## test-unit: run only the tests that need neither docker nor a database
test-unit:
	go test ./internal/cli/ ./internal/config/ ./internal/admin/

## check: format, vet, generated files and test — what CI should run
check: fmt vet generated test

## generated: fail if the committed generated files are stale
#
# This exists because stale generated files fail silently and misleadingly. A
# stale template renders old markup with no error; a stale stylesheet drops
# styles for classes plainly written in the template. You would suspect htmx,
# or Tailwind, or the browser. This turns the whole class into a failure that
# names itself (ADR-0013).
generated: generate
	@git diff --exit-code --stat -- '*_templ.go' internal/admin/static/app.css \
		|| { echo; echo "generated files are stale: run 'make generate' and commit the result"; exit 1; }

## fmt: rewrite sources with gofmt
fmt:
	gofmt -w .

## vet: run go vet over every package
vet:
	go vet ./...

## tidy: prune and record dependencies
tidy:
	go mod tidy

## clean: remove build output
clean:
	rm -rf $(BUILD_DIR)

.PHONY: help build run config generate templ css watch \
        hash-password session-secret \
        db-up db-down db-reset psql \
        migrate migrate-down migrate-status migration \
        test test-unit check generated fmt vet tidy clean
