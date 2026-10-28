# Satu pintu masuk perintah untuk dev, CI, dan deploy.
# Jalankan `make` atau `make help` untuk daftar target.

# Di Windows, resep dijalankan dengan sh dari Git for Windows agar sama dengan Linux/CI,
# baik make dipanggil dari Git Bash maupun PowerShell.
ifeq ($(OS),Windows_NT)
  GIT_SH := $(firstword $(wildcard C:/PROGRA~1/Git/usr/bin/sh.exe C:/PROGRA~1/Git/bin/sh.exe))
  ifneq ($(GIT_SH),)
    SHELL := $(GIT_SH)
    export PATH := $(patsubst %/,%,$(dir $(GIT_SH)));$(PATH)
  endif
endif

VERSION      ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
REGISTRY     ?= ghcr.io/indr622/workflow-engine
PROFILES     ?=
DATABASE_URL ?= postgres://workflow:workflow@localhost:5433/workflow?sslmode=disable
POSTGRES_PASSWORD ?= workflow-local
WEB_PORT     ?= 8088

export VERSION REGISTRY DATABASE_URL POSTGRES_PASSWORD WEB_PORT

NX           := pnpm nx
COMPOSE_DEV  := docker compose -f infra/docker/docker-compose.yml
COMPOSE_PROD := docker compose -f infra/docker/docker-compose.prod.yml
PROFILE_FLAGS := $(foreach p,$(PROFILES),--profile $(p))
MIGRATIONS   := apps/engine/internal/platform/db/migrations

# Versi tool Go yang dipin agar lokal dan CI identik.
BUF_VERSION           := v1.75.0
PROTOC_GEN_GO_VERSION := v1.36.12
CONNECT_GO_VERSION    := v1.21.1
GOOSE_VERSION         := v3.29.0
SQLC_VERSION          := v1.31.1
GOLANGCI_LINT_VERSION := v2.15.0

.DEFAULT_GOAL := help

.PHONY: help setup tools up down logs dev gen lint test build typecheck check test-integration \
        test-race fuzz bench prop parity \
        migrate-up migrate-down migrate-status migrate-new docker-build docker-push \
        deploy-local deploy-local-down clean

help: ## Tampilkan daftar target
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

setup: tools ## Install dependensi Node dan Go, tool Go, dan git hooks
	pnpm install
	go mod download

tools: ## Install tool Go (buf, plugin protoc, goose, sqlc, golangci-lint)
	go install github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)
	go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	go install connectrpc.com/connect/cmd/protoc-gen-connect-go@$(CONNECT_GO_VERSION)
	go install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

up: ## Nyalakan dependensi dev (PROFILES="tools observability" untuk profil tambahan)
	$(COMPOSE_DEV) $(PROFILE_FLAGS) up -d --wait

down: ## Matikan dependensi dev
	$(COMPOSE_DEV) $(PROFILE_FLAGS) down

logs: ## Ikuti log dependensi dev
	$(COMPOSE_DEV) logs -f

dev: ## Jalankan engine dan modeler-web dalam mode dev
	$(NX) run-many -t serve -p engine modeler-web

gen: ## Generate kode dari proto (dan sqlc bila sudah dikonfigurasi)
	$(NX) run proto:generate --skip-nx-cache
	@if [ -f sqlc.yaml ]; then sqlc generate; fi

lint: ## Lint semua proyek (golangci-lint, ESLint, buf lint)
	$(NX) run-many -t lint

test: ## Test semua proyek
	$(NX) run-many -t test

build: ## Build semua proyek
	$(NX) run-many -t build

typecheck: ## Typecheck semua proyek TypeScript
	$(NX) run-many -t typecheck

check: ## Lint, test, typecheck, dan build untuk proyek yang terdampak (dipakai CI)
	$(NX) affected -t lint test typecheck build

test-integration: ## Test integrasi Go terhadap Postgres di DATABASE_URL
	go test -count=1 -tags=integration ./apps/engine/...

test-race: ## Test Go dengan race detector (butuh cgo; dijalankan CI di Linux)
	go test -race -count=1 ./libs/go/... ./apps/engine/...

fuzz: ## Fuzz kernel Go: TARGET=bpmn|feel (default bpmn), FUZZTIME=10m untuk sesi panjang
	@case "$(or $(TARGET),bpmn)" in 		bpmn) go test -run='^$$' -fuzz=FuzzParse -fuzztime=$(or $(FUZZTIME),60s) ./libs/go/bpmn ;; 		feel) go test -run='^$$' -fuzz=FuzzCompile -fuzztime=$(or $(FUZZTIME),60s) ./libs/go/feel ;; 		*) echo "unknown TARGET=$(TARGET) (bpmn|feel)"; exit 1 ;; 	esac

bench: ## Benchmark parser BPMN dan Processor eksekusi
	$(NX) run-many -t bench -p bpmn-go engine

prop: ## Property test gateway: PROP_ITER=100000 (default), PROP_SEED=... untuk mengulang kasus gagal
	PROP_ITER=$(or $(PROP_ITER),100000) PROP_SEED=$(PROP_SEED) go test -run=TestGatewayProperties -count=1 -timeout 30m ./apps/engine/internal/modules/execution/domain

parity: ## Paritas dengan Camunda 8.8: SUITE=deploy|execution|all (default all), LIVE=1 merekam ulang dari Camunda
	@suite="$(or $(SUITE),all)"; 	if [ "$$suite" = "deploy" ] || [ "$$suite" = "all" ]; then 		if [ "$(LIVE)" = "1" ]; then go run ./tools/scripts/parity -live; else go run ./tools/scripts/parity; fi || exit 1; 	fi; 	if [ "$$suite" = "execution" ] || [ "$$suite" = "all" ]; then 		if [ "$(LIVE)" = "1" ]; then 			go test -count=1 -tags=parity -run TestExecutionParityLive -timeout 15m ./apps/engine/internal/modules/execution/domain; 		else 			go test -count=1 -run TestExecutionParity ./apps/engine/internal/modules/execution/domain; 		fi; 	fi

migrate-up: ## Jalankan migrasi ke DATABASE_URL
	goose -dir $(MIGRATIONS) postgres "$(DATABASE_URL)" up

migrate-down: ## Mundurkan satu migrasi
	goose -dir $(MIGRATIONS) postgres "$(DATABASE_URL)" down

migrate-status: ## Tampilkan status migrasi
	goose -dir $(MIGRATIONS) postgres "$(DATABASE_URL)" status

migrate-new: ## Buat migrasi baru: make migrate-new name=add_jobs
	@if [ -z "$(name)" ]; then echo "usage: make migrate-new name=<snake_case>"; exit 1; fi
	goose -dir $(MIGRATIONS) -s create $(name) sql

docker-build: ## Build image engine dan web dengan tag VERSION
	docker build -f infra/docker/engine.Dockerfile --build-arg VERSION=$(VERSION) -t $(REGISTRY)/engine:$(VERSION) .
	docker build -f infra/docker/web.Dockerfile -t $(REGISTRY)/web:$(VERSION) .

docker-push: ## Push image engine dan web ke REGISTRY
	docker push $(REGISTRY)/engine:$(VERSION)
	docker push $(REGISTRY)/web:$(VERSION)

deploy-local: docker-build ## Build image lalu jalankan compose produksi di mesin ini
	$(COMPOSE_PROD) up -d --wait
	@echo "web: http://localhost:$(WEB_PORT)"

deploy-local-down: ## Matikan compose produksi lokal
	$(COMPOSE_PROD) down

clean: ## Hapus hasil build dan cache Nx
	rm -rf dist tmp coverage .nx/cache
