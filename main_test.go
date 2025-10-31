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
