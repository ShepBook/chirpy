package main

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// Test_apiConfig_Initialization verifies that apiConfig can be created
// and fileserverHits starts at 0
func Test_apiConfig_Initialization(t *testing.T) {
	cfg := apiConfig{}

	got := cfg.fileserverHits.Load()
	want := int32(0)

	if got != want {
		t.Errorf("fileserverHits initial value = %d, want %d", got, want)
	}
}

// Test_middlewareMetricsInc_IncrementsCounter verifies middleware increments counter on each request
func Test_middlewareMetricsInc_IncrementsCounter(t *testing.T) {
	cfg := apiConfig{}

	// Create a simple handler that does nothing
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap the handler with middleware
	wrappedHandler := cfg.middlewareMetricsInc(testHandler)

	// Make first request
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec1 := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rec1, req1)

	got := cfg.fileserverHits.Load()
	want := int32(1)
	if got != want {
		t.Errorf("After 1 request: fileserverHits = %d, want %d", got, want)
	}

	// Make second request
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec2 := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rec2, req2)

	got = cfg.fileserverHits.Load()
	want = int32(2)
	if got != want {
		t.Errorf("After 2 requests: fileserverHits = %d, want %d", got, want)
	}
}

// Test_middlewareMetricsInc_CallsNextHandler verifies middleware calls the wrapped handler
func Test_middlewareMetricsInc_CallsNextHandler(t *testing.T) {
	cfg := apiConfig{}

	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("handler executed"))
	})

	wrappedHandler := cfg.middlewareMetricsInc(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Error("Middleware did not call the wrapped handler")
	}

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	if rec.Body.String() != "handler executed" {
		t.Errorf("Response body = %q, want %q", rec.Body.String(), "handler executed")
	}
}

// Test_middlewareMetricsInc_ConcurrentRequests verifies thread-safe increments with concurrent requests
func Test_middlewareMetricsInc_ConcurrentRequests(t *testing.T) {
	cfg := apiConfig{}

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := cfg.middlewareMetricsInc(testHandler)

	// Make 100 concurrent requests
	numRequests := 100
	var wg sync.WaitGroup
	wg.Add(numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rec, req)
		}()
	}

	wg.Wait()

	got := cfg.fileserverHits.Load()
	want := int32(numRequests)
	if got != want {
		t.Errorf("After %d concurrent requests: fileserverHits = %d, want %d", numRequests, got, want)
	}
}

// Test_handlerMetrics_ReturnsPlainText verifies response has Content-Type: text/plain and HTTP 200
func Test_handlerMetrics_ReturnsPlainText(t *testing.T) {
	cfg := apiConfig{}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	cfg.handlerMetrics(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/plain" && contentType != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q or %q", contentType, "text/plain", "text/plain; charset=utf-8")
	}
}

// Test_handlerMetrics_ReturnsCorrectFormat verifies response format is exactly "Hits: x"
func Test_handlerMetrics_ReturnsCorrectFormat(t *testing.T) {
	cfg := apiConfig{}
	cfg.fileserverHits.Store(42)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	cfg.handlerMetrics(rec, req)

	got := rec.Body.String()
	want := "Hits: 42"
	if got != want {
		t.Errorf("Response body = %q, want %q", got, want)
	}
}

// Test_handlerMetrics_ReflectsActualCount verifies displayed count matches actual counter value
func Test_handlerMetrics_ReflectsActualCount(t *testing.T) {
	testCases := []struct {
		name  string
		count int32
		want  string
	}{
		{"zero hits", 0, "Hits: 0"},
		{"one hit", 1, "Hits: 1"},
		{"multiple hits", 123, "Hits: 123"},
		{"large number", 99999, "Hits: 99999"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := apiConfig{}
			cfg.fileserverHits.Store(tc.count)

			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			rec := httptest.NewRecorder()

			cfg.handlerMetrics(rec, req)

			got := rec.Body.String()
			if got != tc.want {
				t.Errorf("With count %d: response body = %q, want %q", tc.count, got, tc.want)
			}

			// Also verify count hasn't changed
			if cfg.fileserverHits.Load() != tc.count {
				t.Errorf("Counter changed from %d to %d", tc.count, cfg.fileserverHits.Load())
			}
		})
	}
}

// Test_handlerReset_ResetsCounter verifies reset endpoint sets counter to 0 using Store(0)
func Test_handlerReset_ResetsCounter(t *testing.T) {
	cfg := apiConfig{}
	cfg.fileserverHits.Store(42)

	req := httptest.NewRequest(http.MethodPost, "/reset", nil)
	rec := httptest.NewRecorder()

	cfg.handlerReset(rec, req)

	got := cfg.fileserverHits.Load()
	want := int32(0)
	if got != want {
		t.Errorf("After reset: fileserverHits = %d, want %d", got, want)
	}
}

// Test_handlerReset_ReturnsSuccess verifies reset endpoint returns HTTP 200
func Test_handlerReset_ReturnsSuccess(t *testing.T) {
	cfg := apiConfig{}
	cfg.fileserverHits.Store(100)

	req := httptest.NewRequest(http.MethodPost, "/reset", nil)
	rec := httptest.NewRecorder()

	cfg.handlerReset(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}
}

// Test_Integration_MetricsWorkflow tests end-to-end workflow: requests -> metrics increment -> reset -> verify zero
func Test_Integration_MetricsWorkflow(t *testing.T) {
	cfg := &apiConfig{}

	// Create a test file server handler
	fileServerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("file content"))
	})

	// Wrap it with metrics middleware
	wrappedHandler := cfg.middlewareMetricsInc(fileServerHandler)

	// Step 1: Make 3 requests to the file server
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/app/index.html", nil)
		rec := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rec, req)
	}

	// Step 2: Check /metrics endpoint shows 3 hits
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	cfg.handlerMetrics(metricsRec, metricsReq)

	metricsBody := metricsRec.Body.String()
	wantMetrics := "Hits: 3"
	if metricsBody != wantMetrics {
		t.Errorf("Metrics before reset = %q, want %q", metricsBody, wantMetrics)
	}

	// Step 3: Call /reset endpoint
	resetReq := httptest.NewRequest(http.MethodPost, "/reset", nil)
	resetRec := httptest.NewRecorder()
	cfg.handlerReset(resetRec, resetReq)

	if resetRec.Code != http.StatusOK {
		t.Errorf("Reset status code = %d, want %d", resetRec.Code, http.StatusOK)
	}

	// Step 4: Verify /metrics now shows 0 hits
	metricsReq2 := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec2 := httptest.NewRecorder()
	cfg.handlerMetrics(metricsRec2, metricsReq2)

	metricsBody2 := metricsRec2.Body.String()
	wantMetricsAfterReset := "Hits: 0"
	if metricsBody2 != wantMetricsAfterReset {
		t.Errorf("Metrics after reset = %q, want %q", metricsBody2, wantMetricsAfterReset)
	}

	// Verify internal counter is actually 0
	if cfg.fileserverHits.Load() != 0 {
		t.Errorf("Internal counter after reset = %d, want 0", cfg.fileserverHits.Load())
	}
}

// Test_methodRestriction_AllowedMethod_CallsHandler verifies that when request method matches allowed method, the wrapped handler is called
func Test_methodRestriction_AllowedMethod_CallsHandler(t *testing.T) {
	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	restrictedHandler := methodRestriction("GET", testHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	restrictedHandler(rec, req)

	if !handlerCalled {
		t.Error("Expected wrapped handler to be called for allowed method")
	}

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	if rec.Body.String() != "success" {
		t.Errorf("Response body = %q, want %q", rec.Body.String(), "success")
	}
}

// Test_methodRestriction_DisallowedMethod_Returns405 verifies that when request method doesn't match, returns HTTP 405
func Test_methodRestriction_DisallowedMethod_Returns405(t *testing.T) {
	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	restrictedHandler := methodRestriction("POST", testHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	restrictedHandler(rec, req)

	if handlerCalled {
		t.Error("Expected wrapped handler NOT to be called for disallowed method")
	}

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// Test_methodRestriction_DisallowedMethod_IncludesAllowHeader verifies that 405 responses include Allow header per RFC 7231
func Test_methodRestriction_DisallowedMethod_IncludesAllowHeader(t *testing.T) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	testCases := []struct {
		name          string
		allowedMethod string
		requestMethod string
	}{
		{"GET allowed, POST attempted", "GET", http.MethodPost},
		{"POST allowed, GET attempted", "POST", http.MethodGet},
		{"DELETE allowed, PUT attempted", "DELETE", http.MethodPut},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			restrictedHandler := methodRestriction(tc.allowedMethod, testHandler)

			req := httptest.NewRequest(tc.requestMethod, "/test", nil)
			rec := httptest.NewRecorder()
			restrictedHandler(rec, req)

			allowHeader := rec.Header().Get("Allow")
			if allowHeader != tc.allowedMethod {
				t.Errorf("Allow header = %q, want %q", allowHeader, tc.allowedMethod)
			}
		})
	}
}

// Test_handlerMetrics_GetRequest_Returns200 verifies that GET request to /metrics returns 200 with metrics data
func Test_handlerMetrics_GetRequest_Returns200(t *testing.T) {
	cfg := &apiConfig{}
	cfg.fileserverHits.Store(5)

	// Create a wrapped handler with method restriction
	wrappedHandler := methodRestriction("GET", cfg.handlerMetrics)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	wrappedHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	expectedBody := "Hits: 5"
	if rec.Body.String() != expectedBody {
		t.Errorf("Response body = %q, want %q", rec.Body.String(), expectedBody)
	}
}

// Test_handlerMetrics_PostRequest_Returns405 verifies that POST request to /metrics returns 405 with Allow header
func Test_handlerMetrics_PostRequest_Returns405(t *testing.T) {
	cfg := &apiConfig{}

	// Create a wrapped handler with method restriction
	wrappedHandler := methodRestriction("GET", cfg.handlerMetrics)

	req := httptest.NewRequest(http.MethodPost, "/metrics", nil)
	rec := httptest.NewRecorder()
	wrappedHandler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}

	allowHeader := rec.Header().Get("Allow")
	if allowHeader != "GET" {
		t.Errorf("Allow header = %q, want %q", allowHeader, "GET")
	}
}

// Test_handlerMetrics_PutRequest_Returns405 verifies that PUT request to /metrics returns 405 with Allow header
func Test_handlerMetrics_PutRequest_Returns405(t *testing.T) {
	cfg := &apiConfig{}

	// Create a wrapped handler with method restriction
	wrappedHandler := methodRestriction("GET", cfg.handlerMetrics)

	req := httptest.NewRequest(http.MethodPut, "/metrics", nil)
	rec := httptest.NewRecorder()
	wrappedHandler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}

	allowHeader := rec.Header().Get("Allow")
	if allowHeader != "GET" {
		t.Errorf("Allow header = %q, want %q", allowHeader, "GET")
	}
}
