#!/bin/bash
set -e

echo "=== Running Unit Tests ==="
go test ./internal/... -v -count=1

echo ""
echo "=== Running go vet ==="
go vet ./...
