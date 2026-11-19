SERVICE_DIR := services/api

.PHONY: api-run api-build api-test api-vet api-fmt migrate-up migrate-down migrate-status compose-up compose-down compose-ps frontend-install frontend-typecheck frontend-build

api-run:
	cd $(SERVICE_DIR) && go run ./cmd/api

api-build:
	cd $(SERVICE_DIR) && go build ./...

api-test:
	cd $(SERVICE_DIR) && go test ./...

api-vet:
	cd $(SERVICE_DIR) && go vet ./...

api-fmt:
	cd $(SERVICE_DIR) && gofmt -w .

migrate-up:
	cd $(SERVICE_DIR) && go run ./cmd/migrate up

migrate-down:
	cd $(SERVICE_DIR) && go run ./cmd/migrate down

migrate-status:
	cd $(SERVICE_DIR) && go run ./cmd/migrate status

seed:
	cd $(SERVICE_DIR) && go run ./cmd/seed

compose-up:
	docker compose -f infra/compose/docker-compose.dev.yml --env-file .env up -d --build

compose-down:
	docker compose -f infra/compose/docker-compose.dev.yml --env-file .env down

compose-ps:
	docker compose -f infra/compose/docker-compose.dev.yml --env-file .env ps

frontend-install:
	pnpm install

frontend-typecheck:
	pnpm -r typecheck

frontend-build:
	pnpm -r build
