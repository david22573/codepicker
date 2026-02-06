#!/bin/bash

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}🔍 Starting CodePicker Health Check...${NC}"
echo "========================================"

# 1. Check for Config File
echo -n "Checking for config.json... "
if [ -f "config.json" ]; then
    echo -e "${GREEN}FOUND${NC}"
else
    echo -e "${RED}MISSING${NC}"
    echo "⚠️  Please create config.json in the project root."
    exit 1
fi

# 2. Tidy Dependencies
echo -n "Tidying module dependencies... "
go mod tidy
if [ $? -eq 0 ]; then
    echo -e "${GREEN}OK${NC}"
else
    echo -e "${RED}FAILED${NC}"
    exit 1
fi

# 3. Build Verification
echo -n "Verifying build... "
go build ./...
if [ $? -eq 0 ]; then
    echo -e "${GREEN}OK${NC}"
else
    echo -e "${RED}FAILED${NC}"
    echo "❌ Compilation error. Check the logs above."
    exit 1
fi

# 4. Run Tests with Race Detection (Crucial for the P0 fix)
echo -e "\n🏃 Running Unit Tests with Race Detector..."
echo "   (This verifies the Circuit Breaker fix)"
go test -race ./...
if [ $? -eq 0 ]; then
    echo -e "\n${GREEN}✅ ALL TESTS PASSED${NC}"
else
    echo -e "\n${RED}❌ TESTS FAILED${NC}"
    exit 1
fi

echo -e "\n${GREEN}🎉 System is Healthy and Production-Ready!${NC}"
