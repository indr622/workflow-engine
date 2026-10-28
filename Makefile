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

# Backup/restore (docs/ops/backup-restore.md). BACKUP_ENV=prod memakai compose produksi;
# BACKUP_ENV=dev memakai compose dev agar runbook bisa dilatih lokal. File compose, nama service,
# dan perintah verifikasi bisa dioverride satu per satu.
BACKUP_ENV          ?= prod
TARGET_DB           ?= workflow_restore
BACKUP_SERVICE      ?= postgres
ifeq ($(BACKUP_ENV),dev)
  COMPOSE_FILE_BACKUP ?= infra/docker/docker-compose.yml
  # Compose dev tidak punya service engine: verifikasi dijalankan native lewat port host.
  RESTORE_URL         ?= postgres://workflow:workflow@localhost:5433/$(TARGET_DB)?sslmode=disable
  RESTORE_VERIFY_CMD  ?= DATABASE_URL="$(RESTORE_URL)" ENGINE_MIGRATE=false go run ./apps/engine/cmd/engine verify --all
else
  COMPOSE_FILE_BACKUP ?= infra/docker/docker-compose.prod.yml
  RESTORE_VERIFY_CMD  ?= $(COMPOSE_BACKUP) run --rm --no-deps -e ENGINE_MIGRATE=false -e DATABASE_URL="postgres://$${POSTGRES_USER:-workflow}:$${POSTGRES_PASSWORD}@postgres:5432/$(TARGET_DB)?sslmode=disable" engine verify --all
endif
# MSYS_NO_PATHCONV mencegah Git Bash di Windows mengubah /backups menjadi path Windows.
COMPOSE_BACKUP      := MSYS_NO_PATHCONV=1 docker compose -f $(COMPOSE_FILE_BACKUP)

# Versi tool Go yang dipin agar lokal dan CI identik.
BUF_VERSION           := v1.75.0
PROTOC_GEN_GO_VERSION := v1.36.12
CONNECT_GO_VERSION    := v1.21.1
GOOSE_VERSION         := v3.29.0
SQLC_VERSION          := v1.31.1
GOLANGCI_LINT_VERSION := v2.15.0

.DEFAULT_GOAL := help

.PHONY: help setup tools up down logs dev gen lint test build typecheck check test-integration \
        test-faultinject test-race fuzz bench prop parity \
        migrate-up migrate-down migrate-status migrate-new migrate-redo docker-build docker-push \
        db-backup db-backup-list db-restore \
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

test-faultinject: ## Test pemulihan dengan fault point (tag integration,faultinject) terhadap DATABASE_URL
	go test -count=1 -tags=integration,faultinject -run 'Crash|Slow|Failure' ./apps/engine/internal/modules/execution/infrastructure/

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

migrate-redo: ## Uji down migration terakhir: goose down lalu up ke DATABASE_URL (jangan di produksi)
	goose -dir $(MIGRATIONS) postgres "$(DATABASE_URL)" down
	goose -dir $(MIGRATIONS) postgres "$(DATABASE_URL)" up

db-backup: ## pg_dump -Fc ke volume postgres-backups (BACKUP_ENV=prod|dev, default prod)
	$(COMPOSE_BACKUP) exec -T $(BACKUP_SERVICE) sh -c 'set -e; f=/backups/workflow-$$(date -u +%Y%m%dT%H%M%SZ).dump; pg_dump -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" -Fc -Z 6 -f "$$f"; ls -l "$$f"'

db-backup-list: ## Daftar file backup di volume postgres-backups (BACKUP_ENV=prod|dev)
	$(COMPOSE_BACKUP) exec -T $(BACKUP_SERVICE) ls -lh /backups

db-restore: ## Restore ke database baru lalu engine verify --all: FILE=workflow-<ts>.dump [TARGET_DB=...]
	@if [ -z "$(FILE)" ]; then echo "usage: make db-restore FILE=workflow-<ts>.dump [TARGET_DB=workflow_restore] [BACKUP_ENV=prod|dev]"; exit 1; fi
	$(COMPOSE_BACKUP) exec -T $(BACKUP_SERVICE) sh -c 'set -e; \
		if [ "$(TARGET_DB)" = "$$POSTGRES_DB" ]; then echo "refusing to restore into the live database $$POSTGRES_DB"; exit 1; fi; \
		test -f "/backups/$(FILE)"; \
		createdb -U "$$POSTGRES_USER" "$(TARGET_DB)"; \
		pg_restore -U "$$POSTGRES_USER" -d "$(TARGET_DB)" --no-owner --exit-on-error "/backups/$(FILE)"; \
		echo "restored /backups/$(FILE) into $(TARGET_DB)"'
	@echo "verifying restored database $(TARGET_DB) (needs the engine verify subcommand)"
	$(RESTORE_VERIFY_CMD)

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

# Suite chaos Compose (Fase 3, 11.2–11.3): image engine dibangun dengan tag yang sama dengan
# `docker-build` (hanya image engine; web tidak dipakai), postgres chaos dinyalakan, runner
# dijalankan, dan project chaos (termasuk volume) selalu dihapus di akhir, juga saat gagal.
COMPOSE_CHAOS      := docker compose -f infra/docker/docker-compose.chaos.yml
CHAOS_ENGINE_IMAGE ?= $(REGISTRY)/engine:$(VERSION)
export CHAOS_ENGINE_IMAGE

.PHONY: chaos

chaos: ## Suite chaos Compose: SCENARIO=all|kill-engine,... (default all), DURATION=30s per skenario
	@trap '$(COMPOSE_CHAOS) --profile replicas down -v --remove-orphans' EXIT; set -e; \
	echo "building $(CHAOS_ENGINE_IMAGE)"; \
	docker build -f infra/docker/engine.Dockerfile --build-arg VERSION=$(VERSION) -t $(CHAOS_ENGINE_IMAGE) .; \
	$(COMPOSE_CHAOS) up -d --wait postgres; \
	go run ./tools/scripts/chaos -scenario "$(or $(SCENARIO),all)" -duration "$(or $(DURATION),30s)" \
		-compose infra/docker/docker-compose.chaos.yml -image "$(CHAOS_ENGINE_IMAGE)"
