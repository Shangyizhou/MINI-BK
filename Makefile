.PHONY: build run test lint clean migrate-up migrate-down

APP_NAME = mini-bk-server
BUILD_DIR = ./bin

build:
	go build -o $(BUILD_DIR)/$(APP_NAME) ./cmd/server

run: build
	$(BUILD_DIR)/$(APP_NAME)

test:
	go test ./... -v -count=1

test-integration:
	go test ./... -v -count=1 -tags=integration

lint:
	golangci-lint run ./...

clean:
	rm -rf $(BUILD_DIR)

migrate-up:
	migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path migrations -database "$(DATABASE_URL)" down

dev: build
	DATABASE_URL=postgres://mini-bk:mini-bk@localhost:5432/mini-bk?sslmode=disable \
	$(BUILD_DIR)/$(APP_NAME)
