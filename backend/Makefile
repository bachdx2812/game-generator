.PHONY: run dev migrate-up migrate-down

DATABASE_URL ?= postgres://postgres:postgres@localhost:54322/specs?sslmode=disable

run:
	go run ./cmd/server

dev:
	go run ./cmd/server

migrate-up:
	migrate -path ./migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path ./migrations -database "$(DATABASE_URL)" down 1
