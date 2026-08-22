# Optional. Only for the local targets: docker compose reads .env on its own.
-include .env
export

.PHONY: help
help:
	@echo targets: run up down redis lint test test/all

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

# ---- tests ----

## test: unit tests
.PHONY: test
test:
	go test ./...

## test/all: unit and integration tests
.PHONY: test/all
test/all:
	go test -tags=integration ./...
