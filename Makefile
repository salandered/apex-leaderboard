# Optional. Only for the local targets: docker compose reads .env on its own.
-include .env
export

# Prints every "## target: description" comment in this file.
.PHONY: help
help:
	@awk '/^## /{sub(/^## /,""); i=index($$0,": "); printf "  %-26s %s\n", substr($$0,1,i-1), substr($$0,i+2)}' $(MAKEFILE_LIST)

# ---- run ----

# .env REDIS_URL points at the compose service name, not reachable from a local run
LOCAL_REDIS_URL ?= redis://localhost:6379/0
run: export REDIS_URL = $(LOCAL_REDIS_URL)

## run: run the app locally, needs redis (see the 'redis' target)
.PHONY: run
run:
	go run .

## up: build and run the whole stack in docker
.PHONY: up
up:
	docker compose up -d --build

## down: stop the stack
.PHONY: down
down:
	docker compose down

## redis: start only redis, for 'make run'
.PHONY: redis
redis:
	docker compose up -d redis

# ---- checks ----

## lint: golangci-lint over both modules
.PHONY: lint
lint:
	golangci-lint run ./...
	cd apiscripts && golangci-lint run ./...

## audit: tidy check, verify, lint and tests over both modules
# -vet=off because golangci-lint already ran govet.
.PHONY: audit
audit:
	@echo Checking module dependencies...
	go mod tidy -diff
	go mod verify
	cd apiscripts && go mod tidy -diff && go mod verify
	@echo Linting...
	golangci-lint run ./...
	cd apiscripts && golangci-lint run ./...
	@echo Running tests...
	go test -race -vet=off ./...

# ---- tests ----

## test: unit tests
.PHONY: test
test:
	go test ./...

## test/all: unit and integration tests
.PHONY: test/all
test/all:
	go test -tags=integration ./...
