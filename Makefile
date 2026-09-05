# Commands used often enough to be worth not retyping. Everything here is a
# thin wrapper: the binary and `go` remain the real interface, and no target
# hides a step you would need to know about when something goes wrong.

BINARY      := personal-web
BUILD_DIR   := bin
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

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
build:
	@mkdir -p $(BUILD_DIR)
	go build -ldflags "-X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY) .

## run: start the HTTP server (migrate first; serve refuses a stale schema)
run:
	go run . serve

## config: print the effective configuration, passwords redacted
config:
	go run . config show

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
	go test ./internal/cli/ ./internal/config/

## check: format, vet and test — what CI should run
check: fmt vet test

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

.PHONY: help build run config db-up db-down db-reset psql \
        migrate migrate-down migrate-status migration \
        test test-unit check fmt vet tidy clean
