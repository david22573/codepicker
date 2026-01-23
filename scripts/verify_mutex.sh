#!/bin/bash
# Verification script for mutex implementation

set -e

echo "🔍 Verifying Mutex Implementation for shadow.Manager"
echo "=================================================="

# Run tests with race detector
echo ""
echo "1️⃣  Running tests with race detector..."
cd internal/shadow
go test -race -v -timeout 30s

echo ""
echo "2️⃣  Running tests with coverage..."
go test -cover -coverprofile=coverage.out

echo ""
echo "3️⃣  Coverage report:"
go tool cover -func=coverage.out | grep -E "^total:|WriteFile|RecordAttribution|LoadManifest|ApplyAtomic"

echo ""
echo "4️⃣  Running stress test (100 concurrent operations)..."
go test -race -run TestConcurrent -count=5

echo ""
echo "✅ All verifications passed!"
echo ""
echo "Summary:"
echo "  - Race detector: PASS"
echo "  - Unit tests: PASS"
echo "  - Coverage: Generated"
echo "  - Stress test: PASS"
echo ""
echo "The shadow.Manager is now thread-safe! 🎉"
