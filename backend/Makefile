.PHONY: run dev migrate-up migrate-down

DB_DSN ?= postgres://postgres:postgres@localhost:54322/specs?sslmode=disable

run:
	go run ./cmd/server

dev:
	go run ./cmd/server

migrate-up:
	migrate -path ./migrations -database "$(DB_DSN)" up

migrate-down:
	migrate -path ./migrations -database "$(DB_DSN)" down 1
