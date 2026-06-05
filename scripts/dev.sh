#!/bin/bash
set -e

echo "=== Mini-BK Dev Setup ==="

# 确保 Docker 可用（macOS 上自动启动 Colima）
if ! docker info >/dev/null 2>&1; then
    if command -v colima &>/dev/null; then
        echo "Docker 不可用，正在启动 Colima..."
        colima start
    else
        echo "Docker 不可用，尝试直接启动后端（跳过容器）..."
        go run ./cmd/server
        exit 0
    fi
fi

# Check if PostgreSQL container exists
if docker ps -a --format '{{.Names}}' | grep -q '^mini-bk-pg$'; then
    if ! docker ps --format '{{.Names}}' | grep -q '^mini-bk-pg$'; then
        echo "Starting existing PostgreSQL container..."
        docker start mini-bk-pg
    else
        echo "PostgreSQL container already running."
    fi
else
    echo "Creating PostgreSQL container..."
    docker run -d --name mini-bk-pg \
        -e POSTGRES_USER=mini-bk \
        -e POSTGRES_PASSWORD=mini-bk \
        -e POSTGRES_DB=mini-bk \
        -p 5432:5432 \
        postgres:16-alpine
fi

# Wait for PG to be ready
echo "Waiting for PostgreSQL to be ready..."
until docker exec mini-bk-pg pg_isready -U mini-bk 2>/dev/null; do
    sleep 1
done

echo "Installing migrate CLI..."
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

echo "Running migrations..."
"$HOME/go/bin/migrate" -path migrations -database "postgres://mini-bk:mini-bk@localhost:5432/mini-bk?sslmode=disable" up

# Build and run
echo "Building and starting server..."
go run ./cmd/server
