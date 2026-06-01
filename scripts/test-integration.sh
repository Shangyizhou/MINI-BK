#!/bin/bash
set -e

echo "=== Running Integration Tests ==="
go test ./internal/integration/ -v -count=1 -tags=integration
