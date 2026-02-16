DATABASE_URL ?= postgres://finny:secret@localhost:5432/finny?sslmode=disable

.PHONY: run build test start stop migrate-up migrate-down lint

run:
	go run cmd/api/main.go

build:
	go build -o bin/api cmd/api/main.go

test:
	go test -v ./...

start:
	docker-compose up -d

stop:
	docker-compose down

migrate-up:
	goose -dir migrations postgres "$(DATABASE_URL)" up

migrate-down:
	goose -dir migrations postgres "$(DATABASE_URL)" down

lint:
	go vet ./...
	gci write -s standard -s default -s "prefix(github.com/MrJamesThe3rd/finny)" . --skip-vendor
	gofumpt -l -w .
	wsl -fix ./... 2> /dev/null || true
	golangci-lint run
	go fmt ./... # This runs last to ensure comment lines have leading spaces
