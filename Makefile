.PHONY: run build migrate migrate-up migrate-down seed test test-integration swagger docker-up docker-down tidy

# ── Local dev ──────────────────────────────────────────────────────────────────
run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

tidy:
	go mod tidy

# ── Database ───────────────────────────────────────────────────────────────────
# Requires migrate CLI: https://github.com/golang-migrate/migrate
DSN := "postgres://mtums:mtums_secret@localhost:5432/mtums_db?sslmode=disable"

migrate: migrate-up

migrate-up:
	migrate -path ./migrations -database $(DSN) up

migrate-down:
	migrate -path ./migrations -database $(DSN) down

migrate-force:
	migrate -path ./migrations -database $(DSN) force $(version)

seed:
	docker compose exec -T postgres psql -U mtums -d mtums_db -f /dev/stdin < migrations/seed.sql

# ── Testing ────────────────────────────────────────────────────────────────────
test:
	go test -v -race ./...

test-integration:
	go test -v -race ./tests/...

# ── Docker ─────────────────────────────────────────────────────────────────────
docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f app

docker-db:
	docker compose exec postgres psql -U mtums -d mtums_db

# ── Swagger ────────────────────────────────────────────────────────────────────
swagger:
	@if command -v swag >/dev/null 2>&1; then \
		swag init -g cmd/server/main.go -o docs; \
	else \
		echo "OpenAPI 3.0 specification available at docs/swagger.yaml"; \
	fi


