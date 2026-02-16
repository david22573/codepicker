# 4. Run Tests (Adaptive)
echo -e "\n🏃 Running Unit Tests..."

# Check if we are on Android/ARM64
ARCH=$(go env GOARCH)
OS=$(go env GOOS)

if [[ "$OS" == "android" && "$ARCH" == "arm64" ]]; then
    echo "   (Skipping -race detector: not supported on android/arm64)"
    TEST_CMD="go test ./..."
else
    echo "   (Including Race Detector)"
    TEST_CMD="go test -race ./..."
fi

$TEST_CMD
if [ $? -eq 0 ]; then
    echo -e "\n${GREEN}✅ ALL TESTS PASSED${NC}"
else
    echo -e "\n${RED}❌ TESTS FAILED${NC}"
    exit 1
fi
