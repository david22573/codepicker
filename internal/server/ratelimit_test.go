package server

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// TestConcurrentRateLimitAllow tests thread-safety of Allow checks
func TestConcurrentRateLimitAllow(t *testing.T) {
	limiter := NewRateLimiter(rate.Limit(100), 10) // 100 req/sec, burst 10

	var wg sync.WaitGroup
	const numWorkers = 20
	const requestsPerWorker = 50

	allowed := make(chan bool, numWorkers*requestsPerWorker)

	// Concurrent workers checking rate limit
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < requestsPerWorker; j++ {
				allow := limiter.globalLimiter.Allow()
				allowed <- allow
			}
		}(i)
	}

	wg.Wait()
	close(allowed)

	// Count how many were allowed vs denied
	allowedCount := 0
	deniedCount := 0
	for a := range allowed {
		if a {
			allowedCount++
		} else {
			deniedCount++
		}
	}

	totalRequests := numWorkers * requestsPerWorker
	if allowedCount+deniedCount != totalRequests {
		t.Errorf("Expected %d total requests, got %d allowed + %d denied = %d",
			totalRequests, allowedCount, deniedCount, allowedCount+deniedCount)
	}

	// With 1000 requests and a limit of 100/sec + burst of 10,
	// most should be denied
	if deniedCount == 0 {
		t.Error("Expected some requests to be denied by rate limiter")
	}

	t.Logf("Rate limiter: %d allowed, %d denied out of %d requests",
		allowedCount, deniedCount, totalRequests)
}

// TestMiddlewareConcurrency tests concurrent HTTP requests through middleware
func TestMiddlewareConcurrency(t *testing.T) {
	limiter := NewRateLimiter(rate.Limit(50), 10)
	middleware := limiter.Middleware()

	// Simple handler that counts requests
	var requestCount int
	var mu sync.Mutex
	handler := func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}

	wrappedHandler := middleware(handler)

	var wg sync.WaitGroup
	const numRequests = 100

	// Track responses
	responses := make(chan int, numRequests)

	// Concurrent HTTP requests
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(reqID int) {
			defer wg.Done()

			req := httptest.NewRequest("GET", "/test", nil)
			rec := httptest.NewRecorder()

			wrappedHandler(rec, req)
			responses <- rec.Code
		}(i)
	}

	wg.Wait()
	close(responses)

	// Count response codes
	okCount := 0
	tooManyCount := 0

	for code := range responses {
		switch code {
		case http.StatusOK:
			okCount++
		case http.StatusTooManyRequests:
			tooManyCount++
		default:
			t.Errorf("Unexpected status code: %d", code)
		}
	}

	if okCount+tooManyCount != numRequests {
		t.Errorf("Expected %d responses, got %d", numRequests, okCount+tooManyCount)
	}

	// Some requests should be rate limited
	if tooManyCount == 0 {
		t.Error("Expected some requests to be rate limited (429)")
	}

	// Handler should only be called for allowed requests
	mu.Lock()
	finalCount := requestCount
	mu.Unlock()

	if finalCount != okCount {
		t.Errorf("Handler called %d times, but %d requests got 200 OK", finalCount, okCount)
	}

	t.Logf("Middleware: %d OK, %d rate limited", okCount, tooManyCount)
}

// TestRetryAfterHeader tests that Retry-After header is set correctly
func TestRetryAfterHeader(t *testing.T) {
	limiter := NewRateLimiter(rate.Limit(1), 1) // Very restrictive
	middleware := limiter.Middleware()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	wrappedHandler := middleware(handler)

	var wg sync.WaitGroup
	const numRequests = 20

	retryAfterValues := make(chan string, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req := httptest.NewRequest("GET", "/test", nil)
			rec := httptest.NewRecorder()

			wrappedHandler(rec, req)

			if rec.Code == http.StatusTooManyRequests {
				retryAfter := rec.Header().Get("Retry-After")
				retryAfterValues <- retryAfter
			}
		}()
	}

	wg.Wait()
	close(retryAfterValues)

	// All rate-limited requests should have Retry-After header
	for retryAfter := range retryAfterValues {
		if retryAfter == "" {
			t.Error("Retry-After header should be set for 429 responses")
		}
		if retryAfter != "60" {
			t.Errorf("Expected Retry-After: 60, got: %s", retryAfter)
		}
	}
}

// TestBurstHandling tests burst capacity handling
func TestBurstHandling(t *testing.T) {
	burstSize := 5
	limiter := NewRateLimiter(rate.Limit(1), burstSize)

	var wg sync.WaitGroup
	const numRequests = 10

	results := make(chan bool, numRequests)

	// Fire all requests simultaneously (testing burst)
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allow := limiter.globalLimiter.Allow()
			results <- allow
		}()
	}

	wg.Wait()
	close(results)

	allowedCount := 0
	for allowed := range results {
		if allowed {
			allowedCount++
		}
	}

	// Should allow burst + 1 (the rate limit token)
	// Actually, burst allows exactly `burstSize` tokens
	if allowedCount < burstSize || allowedCount > burstSize+1 {
		t.Errorf("Expected around %d requests allowed (burst capacity), got %d",
			burstSize, allowedCount)
	}

	t.Logf("Burst test: %d/%d requests allowed (burst size: %d)",
		allowedCount, numRequests, burstSize)
}

// TestRateLimiterRecovery tests rate limiter recovery over time
func TestRateLimiterRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping time-dependent test in short mode")
	}

	// 10 requests per second, burst of 2
	limiter := NewRateLimiter(rate.Limit(10), 2)

	// Exhaust the burst
	for i := 0; i < 3; i++ {
		limiter.globalLimiter.Allow()
	}

	// Should be rate limited now
	if limiter.globalLimiter.Allow() {
		t.Error("Expected to be rate limited after exhausting burst")
	}

	// Wait for recovery (100ms should give 1 token at 10/sec rate)
	time.Sleep(150 * time.Millisecond)

	// Should allow at least one more request
	if !limiter.globalLimiter.Allow() {
		t.Error("Expected rate limiter to recover after waiting")
	}
}

// TestConcurrentMiddlewareChains tests multiple middleware instances
func TestConcurrentMiddlewareChains(t *testing.T) {
	limiter := NewRateLimiter(rate.Limit(100), 20)
	middleware := limiter.Middleware()

	// Create a chain with multiple middlewares
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	// Wrap with the same middleware multiple times (not typical, but tests thread safety)
	wrappedHandler := middleware(middleware(handler))

	var wg sync.WaitGroup
	const numRequests = 50

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req := httptest.NewRequest("GET", "/test", nil)
			rec := httptest.NewRecorder()

			wrappedHandler(rec, req)

			// Just verify it doesn't panic or deadlock
			if rec.Code != http.StatusOK && rec.Code != http.StatusTooManyRequests {
				t.Errorf("Unexpected status code: %d", rec.Code)
			}
		}()
	}

	wg.Wait()
}

// TestRaceDetection runs rapid concurrent operations (run with -race flag)
func TestRaceDetection(t *testing.T) {
	limiter := NewRateLimiter(rate.Limit(1000), 100)
	middleware := limiter.Middleware()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	wrappedHandler := middleware(handler)

	var wg sync.WaitGroup
	const iterations = 200

	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Mix direct Allow calls with middleware calls
			if i%2 == 0 {
				limiter.globalLimiter.Allow()
			} else {
				req := httptest.NewRequest("GET", "/test", nil)
				rec := httptest.NewRecorder()
				wrappedHandler(rec, req)
			}
		}()
	}

	wg.Wait()
}

// TestHighConcurrencyStress tests the limiter under high load
func TestHighConcurrencyStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	limiter := NewRateLimiter(rate.Limit(1000), 50)
	middleware := limiter.Middleware()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	wrappedHandler := middleware(handler)

	var wg sync.WaitGroup
	const numWorkers = 50
	const requestsPerWorker = 100

	startTime := time.Now()

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < requestsPerWorker; j++ {
				req := httptest.NewRequest("GET", "/test", nil)
				rec := httptest.NewRecorder()
				wrappedHandler(rec, req)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	totalRequests := numWorkers * requestsPerWorker
	requestsPerSecond := float64(totalRequests) / duration.Seconds()

	t.Logf("Processed %d requests in %v (%.2f req/sec)",
		totalRequests, duration, requestsPerSecond)
}

// TestConcurrentLimiterCreation tests creating multiple limiters concurrently
func TestConcurrentLimiterCreation(t *testing.T) {
	var wg sync.WaitGroup
	const numLimiters = 20

	limiters := make([]*RateLimiter, numLimiters)

	for i := 0; i < numLimiters; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			limiters[idx] = NewRateLimiter(rate.Limit(100), 10)
		}(i)
	}

	wg.Wait()

	// Verify all limiters were created
	for i, limiter := range limiters {
		if limiter == nil {
			t.Errorf("Limiter %d was not created", i)
		}
	}
}

// TestMiddlewareWithDifferentPaths tests concurrent requests to different endpoints
func TestMiddlewareWithDifferentPaths(t *testing.T) {
	limiter := NewRateLimiter(rate.Limit(50), 10)
	middleware := limiter.Middleware()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Path: " + r.URL.Path))
	}

	wrappedHandler := middleware(handler)

	var wg sync.WaitGroup
	paths := []string{"/api/users", "/api/posts", "/api/comments", "/health", "/metrics"}
	const requestsPerPath = 20

	for _, path := range paths {
		for i := 0; i < requestsPerPath; i++ {
			wg.Add(1)
			go func(p string) {
				defer wg.Done()

				req := httptest.NewRequest("GET", p, nil)
				rec := httptest.NewRecorder()

				wrappedHandler(rec, req)

				// Verify response is valid
				if rec.Code != http.StatusOK && rec.Code != http.StatusTooManyRequests {
					t.Errorf("Unexpected status for path %s: %d", p, rec.Code)
				}
			}(path)
		}
	}

	wg.Wait()
}

// TestZeroRateLimit tests behavior with zero rate limit
func TestZeroRateLimit(t *testing.T) {
	// Rate limit of 0 should block everything except burst
	limiter := NewRateLimiter(rate.Limit(0), 5)
	middleware := limiter.Middleware()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	wrappedHandler := middleware(handler)

	var wg sync.WaitGroup
	const numRequests = 20

	okCount := 0
	var mu sync.Mutex

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req := httptest.NewRequest("GET", "/test", nil)
			rec := httptest.NewRecorder()

			wrappedHandler(rec, req)

			if rec.Code == http.StatusOK {
				mu.Lock()
				okCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// With rate 0, only burst tokens (5) should be allowed
	if okCount > 5 {
		t.Errorf("Expected at most 5 requests allowed with zero rate, got %d", okCount)
	}

	t.Logf("Zero rate test: %d/%d allowed (burst: 5)", okCount, numRequests)
}

// TestConcurrentReservations tests concurrent Reserve operations
func TestConcurrentReservations(t *testing.T) {
	limiter := NewRateLimiter(rate.Limit(100), 20)

	var wg sync.WaitGroup
	const numReservations = 50

	reservations := make([]*rate.Reservation, numReservations)

	// Concurrent reservations
	for i := 0; i < numReservations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			reservations[idx] = limiter.globalLimiter.Reserve()
		}(i)
	}

	wg.Wait()

	// Verify all reservations were made
	for i, res := range reservations {
		if res == nil {
			t.Errorf("Reservation %d was nil", i)
		}
	}

	// Cancel all reservations
	for _, res := range reservations {
		if res != nil {
			res.Cancel()
		}
	}
}
