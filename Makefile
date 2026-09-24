ifeq (,$(filter test test-integration,$(MAKECMDGOALS)))
ifneq (,$(wildcard .env))
include .env
export
endif
endif

GO ?= go
DOCKER ?= docker
COMPOSE ?= $(DOCKER) compose
GOCACHE ?= /tmp/go-build-cache
GOMODCACHE ?= /tmp/go-mod-cache
GO_ENV = env GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE)
DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/crowdbeats?sslmode=disable

.PHONY: build dev test test-unit test-integration fmt tidy docker-build up down logs

build:
	$(GO_ENV) $(GO) build ./...

dev:
	$(GO_ENV) $(GO) run ./cmd/api

test: test-unit test-integration

test-unit:
	$(GO_ENV) $(GO) test ./...

test-integration:
	@test -n "$(TEST_DATABASE_URL)" || { echo "TEST_DATABASE_URL is required for integration tests"; exit 1; }
	@test "$(ALLOW_INTEGRATION_DB_RESET)" = "true" || { echo "ALLOW_INTEGRATION_DB_RESET=true is required for integration tests"; exit 1; }
	$(GO_ENV) $(GO) test -count=1 -tags=integration ./...

fmt:
	$(GO_ENV) $(GO) fmt ./...

tidy:
	$(GO_ENV) $(GO) mod tidy

docker-build:
	$(DOCKER) build -t crowd-beats-api:local .

up:
	$(COMPOSE) up -d --build

down:
	$(COMPOSE) down -v

logs:
	$(COMPOSE) logs -f api postgres
