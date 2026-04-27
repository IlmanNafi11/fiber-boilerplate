.PHONY: migrate-up migrate-down migrate-down-all migrate-version migrate-force

migrate-up:
	go run ./cmd/migrate/main.go up

migrate-down:
	go run ./cmd/migrate/main.go down

migrate-down-all:
	go run ./cmd/migrate/main.go down -all

migrate-version:
	go run ./cmd/migrate/main.go version

migrate-force:
	go run ./cmd/migrate/main.go force -version $(V)
