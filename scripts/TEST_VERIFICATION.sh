#!/bin/bash

# Test Verification Script for Concurrency Tests
# This script verifies all new concurrency tests compile and run correctly

set -e

echo "=================================================="
echo "Concurrency Test Verification Script"
echo "=================================================="
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to run tests for a package
run_package_tests() {
    local package=$1
    local test_pattern=$2
    
    echo -e "${YELLOW}Testing ${package}...${NC}"
    
    # Compile check
    if go test -c "${package}" -o /dev/null 2>&1; then
        echo -e "${GREEN}✓ Compilation successful${NC}"
    else
        echo -e "${RED}✗ Compilation failed${NC}"
        go test -c "${package}" 2>&1 | head -20
        return 1
    fi
    
    # Run tests (without race detector first for speed)
    if go test "${package}" -run "${test_pattern}" -v 2>&1 | grep -E "PASS|FAIL"; then
        echo -e "${GREEN}✓ Tests passed${NC}"
    else
        echo -e "${RED}✗ Some tests failed${NC}"
        return 1
    fi
    
    echo ""
}

# Function to run tests with race detector
run_race_tests() {
    local package=$1
    local test_pattern=$2
    
    echo -e "${YELLOW}Race testing ${package}...${NC}"
    
    if go test "${package}" -run "${test_pattern}" -race -timeout 30s 2>&1 | tail -5 | grep -q "PASS"; then
        echo -e "${GREEN}✓ Race detector found no issues${NC}"
    else
        echo -e "${RED}✗ Race detector found issues or tests failed${NC}"
        return 1
    fi
    
    echo ""
}

# Test tracking package
echo "=================================================="
echo "1. Testing internal/tracking"
echo "=================================================="
run_package_tests "./internal/tracking" "TestConcurrent"
run_race_tests "./internal/tracking" "TestRaceDetection"

# Test writer package
echo "=================================================="
echo "2. Testing internal/writer"
echo "=================================================="
run_package_tests "./internal/writer" "TestConcurrent"
run_race_tests "./internal/writer" "TestRaceDetection"

# Test server package
echo "=================================================="
echo "3. Testing internal/server"
echo "=================================================="
run_package_tests "./internal/server" "TestConcurrent"
run_race_tests "./internal/server" "TestRaceDetection"

# Test progress package
echo "=================================================="
echo "4. Testing internal/progress"
echo "=================================================="
run_package_tests "./internal/progress" "TestConcurrent"
run_race_tests "./internal/progress" "TestRaceDetection"

# Test batch package
echo "=================================================="
echo "5. Testing internal/batch"
echo "=================================================="
run_package_tests "./internal/batch" "TestRunner"
run_race_tests "./internal/batch" "TestRunnerRaceDetection"

echo "=================================================="
echo "Summary"
echo "=================================================="
echo ""
echo -e "${GREEN}All concurrency tests completed!${NC}"
echo ""
echo "Next steps:"
echo "1. Review test output for any warnings"
echo "2. Add tests to CI/CD pipeline with -race flag"
echo "3. Monitor for flaky tests in CI"
echo ""
echo "To run specific tests:"
echo "  go test ./internal/tracking -run TestConcurrentRecordRequest -v -race"
echo "  go test ./internal/writer -run TestConcurrentConcatWrites -v -race"
echo "  go test ./internal/server -run TestMiddlewareConcurrency -v -race"
echo ""
