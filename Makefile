.PHONY: run build migrate-up migrate-down seed test swagger docker-up docker-down tidy

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
	go test -v -race -tags=integration ./tests/...

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
	swag init -g cmd/server/main.go -o docs

