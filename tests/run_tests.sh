#!/bin/bash

echo "🧪 Starting CodePicker Integration Suite..."
echo "=========================================="

# Run tests with verbose output to see the checks passing
# -count=1 disables caching so we always run fresh
go test -v -count=1 ./tests/integration/...

EXIT_CODE=$?

echo "=========================================="
if [ $EXIT_CODE -eq 0 ]; then
    echo "✅ ALL SYSTEMS GREEN. RELEASE CANDIDATE READY."
else
    echo "❌ TESTS FAILED. DO NOT DEPLOY."
fi

exit $EXIT_CODE
