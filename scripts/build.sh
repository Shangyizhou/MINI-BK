#!/bin/bash
set -e

echo "=== Building mini-bk-server ==="
go build -o bin/mini-bk-server ./cmd/server
echo "Binary: bin/mini-bk-server"
