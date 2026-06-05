#!/bin/bash
set -e

echo "=== Mini-BK Web Console ==="

cd "$(dirname "$0")/../web"

if [ ! -d "node_modules" ]; then
    echo "Installing dependencies..."
    npm install
fi

echo "Starting frontend dev server..."
npm run dev
