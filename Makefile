ifneq (,$(wildcard .env))
include .env
export
endif

GO ?= go
DOCKER ?= docker
COMPOSE ?= $(DOCKER) compose
GOCACHE ?= /tmp/go-build-cache
GOMODCACHE ?= /tmp/go-mod-cache
GO_ENV = env GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE)
DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/crowdbeats?sslmode=disable
TEST_DATABASE_URL ?= $(DATABASE_URL)

.PHONY: build dev test test-unit test-integration fmt tidy docker-build up down logs

build:
	$(GO_ENV) $(GO) build ./...

dev:
	$(GO_ENV) $(GO) run ./cmd/api

test: test-unit test-integration

test-unit:
	$(GO_ENV) $(GO) test ./...

test-integration:
	TEST_DATABASE_URL=$(TEST_DATABASE_URL) $(GO_ENV) $(GO) test -tags=integration ./...

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
