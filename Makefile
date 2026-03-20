.PHONY: build test lint migrate-up migrate-down sqlc proto dev dev-down dev-logs clean css css-watch

COMPOSE_FILE := deploy/docker/docker-compose.yml

build:
	go build -o bin/server ./cmd/server
	go build -o bin/worker ./cmd/worker
	go build -o bin/backtest ./cmd/backtest

test:
	go test ./...

lint:
	golangci-lint run ./...

sqlc:
	sqlc generate -f sql/sqlc.yaml

migrate-up:
	migrate -path migrations -database "$$BETBOT_DATABASE_URL" up

migrate-down:
	migrate -path migrations -database "$$BETBOT_DATABASE_URL" down 1

proto:
	buf generate

dev:
	docker compose -f $(COMPOSE_FILE) up -d --build

dev-down:
	docker compose -f $(COMPOSE_FILE) down

dev-logs:
	docker compose -f $(COMPOSE_FILE) logs -f betbot postgres

css: ## Build minified CSS
	npx @tailwindcss/cli -i static/css/main.css -o static/css/out.css --minify

css-watch: ## Watch templates + CSS for changes
	npx @tailwindcss/cli -i static/css/main.css -o static/css/out.css --watch

clean:
	rm -rf bin/

