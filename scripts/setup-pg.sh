#!/bin/bash
set -e

echo "=== Setting up PostgreSQL ==="

# 确保 Docker 可用（macOS 上自动启动 Colima）
if ! docker info >/dev/null 2>&1; then
    if command -v colima &>/dev/null; then
        echo "Docker 不可用，正在启动 Colima..."
        colima start
    else
        echo "错误: Docker 不可用，请安装 Docker Desktop 或 Colima"
        exit 1
    fi
fi

if docker ps -a --format '{{.Names}}' | grep -q '^mini-bk-pg$'; then
    echo "Removing old PostgreSQL container..."
    docker rm -f mini-bk-pg
fi

echo "Creating PostgreSQL container..."
docker run -d --name mini-bk-pg \
    -e POSTGRES_USER=mini-bk \
    -e POSTGRES_PASSWORD=mini-bk \
    -e POSTGRES_DB=mini-bk \
    -p 5432:5432 \
    postgres:16-alpine

echo "Waiting for PostgreSQL to be ready..."
until docker exec mini-bk-pg pg_isready -U mini-bk 2>/dev/null; do
    sleep 1
done

echo "Installing migrate CLI..."
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

echo "Running migrations..."
"$HOME/go/bin/migrate" -path migrations -database "postgres://mini-bk:mini-bk@localhost:5432/mini-bk?sslmode=disable" up

echo "PostgreSQL is ready!"

echo ""
echo "=== Setting up Redis ==="
if docker ps -a --format '{{.Names}}' | grep -q '^mini-bk-redis$'; then
    echo "Removing old Redis container..."
    docker rm -f mini-bk-redis
fi

echo "Creating Redis container..."
docker run -d --name mini-bk-redis -p 6379:6379 redis:7-alpine

echo "Waiting for Redis to be ready..."
until docker exec mini-bk-redis redis-cli ping 2>/dev/null | grep -q 'PONG'; do
    sleep 1
done

echo "Redis is ready!"
