#!/bin/bash

# NexusCache Demo Script
# This script demonstrates the distributed cache functionality

set -e

echo "=========================================="
echo "  NexusCache Distributed Cache Demo"
echo "=========================================="
echo ""

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Wait for services to be ready
wait_for_services() {
    echo -e "${YELLOW}Waiting for services to be ready...${NC}"
    sleep 10
    
    # Check if svc1 is responding
    for i in {1..30}; do
        if curl -s "http://localhost:9999/api/get?key=test" > /dev/null 2>&1; then
            echo -e "${GREEN}Services are ready!${NC}"
            return 0
        fi
        echo "Waiting... ($i/30)"
        sleep 2
    done
    echo "Services failed to start!"
    exit 1
}

# Demo: Set and Get values
demo_basic_operations() {
    echo ""
    echo -e "${BLUE}=== Demo 1: Basic SET/GET Operations ===${NC}"
    echo ""
    
    # Set a value
    echo "Setting key 'user1' with value 'John Doe', expiry 5 min..."
    curl -s -X POST "http://localhost:9999/api/set" \
        -d "key=user1&value=John Doe&expire=5&hot=false"
    echo ""
    
    # Get the value
    echo "Getting key 'user1'..."
    curl -s "http://localhost:9999/api/get?key=user1"
    echo ""
}

# Demo: Test distributed nature
demo_distributed() {
    echo ""
    echo -e "${BLUE}=== Demo 2: Distributed Cache ===${NC}"
    echo ""
    
    # Set values across different keys (will be distributed to different nodes)
    echo "Setting multiple keys (distributed across nodes)..."
    for i in {1..5}; do
        curl -s -X POST "http://localhost:9999/api/set" \
            -d "key=item$i&value=Value$i&expire=5&hot=false" > /dev/null
        echo "  Set item$i = Value$i"
    done
    
    echo ""
    echo "Getting values (may come from different nodes)..."
    for i in {1..5}; do
        result=$(curl -s "http://localhost:9999/api/get?key=item$i")
        echo "  $result"
    done
}

# Demo: Hot cache
demo_hot_cache() {
    echo ""
    echo -e "${BLUE}=== Demo 3: Hot Cache (Replicated Data) ===${NC}"
    echo ""
    
    echo "Setting 'popular_item' as HOT data (replicated to all nodes)..."
    curl -s -X POST "http://localhost:9999/api/set" \
        -d "key=popular_item&value=This is hot data!&expire=5&hot=true"
    echo ""
    
    echo "Getting 'popular_item' from Node 1 (port 9999)..."
    curl -s "http://localhost:9999/api/get?key=popular_item"
    echo ""
    
    echo "Getting 'popular_item' from Node 2 (port 9998)..."
    curl -s "http://localhost:9998/api/get?key=popular_item"
    echo ""
    
    echo "Getting 'popular_item' from Node 3 (port 9997)..."
    curl -s "http://localhost:9997/api/get?key=popular_item"
    echo ""
}

# Demo: Built-in test data
demo_builtin_data() {
    echo ""
    echo -e "${BLUE}=== Demo 4: Built-in Test Data ===${NC}"
    echo ""
    
    echo "Getting pre-defined test data (Tom, Jack, Sam)..."
    echo "  Tom: $(curl -s 'http://localhost:9999/api/get?key=Tom')"
    echo "  Jack: $(curl -s 'http://localhost:9999/api/get?key=Jack')"
    echo "  Sam: $(curl -s 'http://localhost:9999/api/get?key=Sam')"
}

# Main
main() {
    wait_for_services
    demo_basic_operations
    demo_distributed
    demo_hot_cache
    demo_builtin_data
    
    echo ""
    echo -e "${GREEN}=========================================="
    echo "  Demo Complete!"
    echo "==========================================${NC}"
    echo ""
    echo "The cache cluster is still running. You can:"
    echo "  - Set values: curl -X POST 'http://localhost:9999/api/set' -d 'key=X&value=Y&expire=5&hot=false'"
    echo "  - Get values: curl 'http://localhost:9999/api/get?key=X'"
    echo "  - Stop cluster: docker-compose down"
    echo ""
}

main
