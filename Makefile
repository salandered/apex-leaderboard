# Default version (the CI release job passes the computed tag)
VERSION ?= dev
# '-s -w' strips symbol tables and DWARF; '-X' for version
LDFLAGS = -s -w -X github.com/salandered/apex/handlers.version=$(VERSION)

# Optional. Only for the local targets: docker compose reads .env on its own.
-include .env
export

.PHONY: help
help:
	@echo targets: run dc/up dc/down web/up db/up lint audit test test/all etc ...

# ==================================================================================== #
# DEVELOPMENT
# ==================================================================================== #

# .env REDIS_URL points at the compose service name, not reachable from a local run
LOCAL_REDIS_URL ?= redis://localhost:6379/0
run: export REDIS_URL = $(LOCAL_REDIS_URL)

## run: run the app locally, needs redis (see the 'db/up' target)
.PHONY: run
run:
	go run .

## dc/up: build and run the stack in docker
.PHONY: dc/up
dc/up:
	docker compose up -d --build

## dc/down: stop the stack
.PHONY: dc/down
dc/down:
	docker compose down

## web/up: run the stack + the web ui, on UI_PORT (8089 by default)
.PHONY: web/up
web/up:
	docker compose -f docker-compose.yml -f docker-compose.web.yml up -d --build

## web/down: stop the stack + web
.PHONY: web/down
web/down:
	docker compose -f docker-compose.yml -f docker-compose.web.yml down

## db/up: start only the redis container
.PHONY: db/up
db/up:
	docker compose up -d redis

## db/cli: connect to redis using redis-cli in its container
.PHONY: db/cli
db/cli:
	@docker compose exec redis redis-cli

# ==================================================================================== #
# QUALITY CONTROL
# ==================================================================================== #

## tidy
.PHONY: tidy
tidy:
	go mod tidy
	cd apiscripts && go mod tidy

## lint
.PHONY: lint
lint:
	golangci-lint run ./...
	cd apiscripts && golangci-lint run ./...

## fmt
.PHONY: fmt
fmt:
	golangci-lint fmt ./...
	cd apiscripts && golangci-lint fmt ./...

## audit: tidy, lint and tests (all modules)
# -vet=off because golangci-lint already uses govet.
.PHONY: audit
audit:
	go mod tidy -diff
	go mod verify
	cd apiscripts && go mod tidy -diff && go mod verify
	golangci-lint run ./...
	cd apiscripts && golangci-lint run ./...
	go test -race -vet=off ./...

# ==================================================================================== #
# TESTS
# ==================================================================================== #

## test: unit tests
.PHONY: test
test:
	go test ./...

## test/all: unit and integration tests, needs Docker
.PHONY: test/all
test/all:
	go test -tags=integration ./...

# ==================================================================================== #
# BUILD
# ==================================================================================== #
# For a local binary

## build: build the app for win and linux
.PHONY: build
build: build/win build/linux

## build/win: build the app for Win
# Windows needs "-o=./bin/apex.exe", not "-o=./bin/apex"
.PHONY: build/win
build/win:
	@echo Building for windows/amd64...
	go build -ldflags='$(LDFLAGS)' -o=./bin/apex.exe .

## build/linux: build the app for Ubuntu Linux
.PHONY: build/linux
# A "VAR=value cmd" not working when running make on Win
build/linux: export GOOS=linux
build/linux: export GOARCH=amd64
build/linux: export CGO_ENABLED=0
build/linux:
	@echo Building for linux/amd64...
	go build -ldflags='$(LDFLAGS)' -o=./bin/linux_amd64/apex .
