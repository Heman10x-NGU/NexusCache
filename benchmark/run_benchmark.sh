#!/bin/bash

# NexusCache Benchmark Script
# Run this after docker-compose up

set -e

echo "=========================================="
echo "  NexusCache Performance Benchmark"
echo "=========================================="
echo ""

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Check if services are running
echo "Checking if services are ready..."
if ! curl -s "http://localhost:9999/api/get?key=test" > /dev/null 2>&1; then
    echo -e "${RED}ERROR: Cache service not responding on localhost:9999${NC}"
    echo "Please run 'docker-compose up -d' first and wait for services to start."
    exit 1
fi

echo -e "${GREEN}Services are ready!${NC}"
echo ""

# Change to script directory
cd "$(dirname "$0")"

# Parse arguments or use defaults
DURATION="${1:-30s}"
CONCURRENCY="${2:-50}"
KEYS="${3:-100}"

echo "Running benchmark..."
echo "  - Duration: $DURATION"
echo "  - Concurrency: $CONCURRENCY workers"
echo "  - Key Space: $KEYS keys"
echo "  - Read Ratio: 80% reads, 20% writes"
echo ""

# Run the benchmark
go run load_test.go \
    -duration="$DURATION" \
    -concurrency="$CONCURRENCY" \
    -keys="$KEYS"

echo ""
echo -e "${GREEN}Benchmark complete!${NC}"
