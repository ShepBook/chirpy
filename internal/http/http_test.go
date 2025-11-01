package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	httpserver "github.com/ShepBook/chirpy/internal/http"
)

// Helper to access unexported cleanProfanity function for testing
// We need to create a test helper in the http package or export the function
// For now, we'll need to change our approach - either export it or use same package

// Phase 1: Constructor Testing

func Test_New_ReturnsServerWithCorrectConfiguration(t *testing.T) {
	// Act
	server := httpserver.New()

	// Assert
	if server == nil {
		t.Fatal("Expected server to be non-nil, got nil")
	}

	// We need to access the internal http.Server to verify configuration
	// Since httpSrv is not exported, we'll verify behavior through the public interface
	// and test the server can be started (which validates initialization)

	// Start server in a goroutine to verify it was properly initialized
	serverErr := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		serverErr <- err
	}()

	// Give server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Verify server is listening by making a request
	resp, err := http.Get("http://localhost:8080")
	if err != nil {
		t.Fatalf("Expected server to be listening on :8080, got error: %v", err)
	}
	resp.Body.Close()

	// Verify we get a 404 (expected since no routes are configured)
	// This confirms the ServeMux is properly initialized
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status code %d, got %d", http.StatusNotFound, resp.StatusCode)
	}

	// Clean up: shutdown the server
	if err := server.Shutdown(context.Background()); err != nil {
		t.Logf("Warning: error during shutdown: %v", err)
	}

	// Wait a moment for shutdown to complete
	time.Sleep(50 * time.Millisecond)
}

// Phase 2: Server Lifecycle Testing

func Test_ListenAndServe_StartsServer(t *testing.T) {
	server := httpserver.New()

	// Start server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Verify server is listening by making a request
	resp, err := http.Get("http://localhost:8080")
	if err != nil {
		t.Fatalf("Expected server to be listening, got error: %v", err)
	}
	defer resp.Body.Close()

	// Verify we get a response (404 is expected with no routes)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status code %d, got %d", http.StatusNotFound, resp.StatusCode)
	}

	// Shutdown server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Errorf("Expected no error during shutdown, got: %v", err)
	}

	// Check if there were any startup errors
	if err := <-serverErr; err != nil {
		t.Errorf("Expected no server errors, got: %v", err)
	}
}

func Test_ListenAndServe_ReturnsErrorOnPortConflict(t *testing.T) {
	// Start first server
	server1 := httpserver.New()
	go func() {
		_ = server1.ListenAndServe()
	}()

	// Give first server time to bind to port
	time.Sleep(100 * time.Millisecond)

	// Try to start second server on same port
	server2 := httpserver.New()
	errChan := make(chan error, 1)
	go func() {
		errChan <- server2.ListenAndServe()
	}()

	// Wait for error from second server
	select {
	case err := <-errChan:
		if err == nil {
			t.Error("Expected error when binding to already-used port, got nil")
		}
		// Error is expected, test passes
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for port conflict error")
	}

	// Cleanup: shutdown first server
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server1.Shutdown(ctx)
}

func Test_Shutdown_GracefulShutdown(t *testing.T) {
	server := httpserver.New()

	// Start server
	go func() {
		_ = server.ListenAndServe()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Verify server is running
	resp, err := http.Get("http://localhost:8080")
	if err != nil {
		t.Fatalf("Expected server to be running, got error: %v", err)
	}
	resp.Body.Close()

	// Shutdown with valid context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = server.Shutdown(ctx)
	if err != nil {
		t.Errorf("Expected graceful shutdown to return nil error, got: %v", err)
	}

	// Verify server is no longer accepting connections
	time.Sleep(100 * time.Millisecond)
	_, err = http.Get("http://localhost:8080")
	if err == nil {
		t.Error("Expected error connecting to shutdown server, got nil")
	}
}

func Test_Shutdown_WithCancelledContext(t *testing.T) {
	server := httpserver.New()

	// Start server
	go func() {
		_ = server.ListenAndServe()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Create an already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Attempt shutdown with cancelled context
	err := server.Shutdown(ctx)
	// Note: Shutdown might succeed if it completes before checking context
	// So we accept either context.Canceled or nil
	if err != nil && err != context.Canceled {
		t.Errorf("Expected nil or context.Canceled error, got: %v", err)
	}

	// Cleanup: force shutdown with valid context if needed
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cleanupCancel()
	_ = server.Shutdown(cleanupCtx)
}

func Test_Shutdown_WithTimeoutContext(t *testing.T) {
	server := httpserver.New()

	// Start server
	go func() {
		_ = server.ListenAndServe()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Create context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to expire
	time.Sleep(10 * time.Millisecond)

	// Attempt shutdown with expired context
	err := server.Shutdown(ctx)
	if err != context.DeadlineExceeded {
		// Note: In practice, the server might shutdown before the timeout
		// So we accept both nil (successful shutdown) or DeadlineExceeded
		if err != nil {
			t.Logf("Got error during shutdown with timeout context: %v", err)
		}
	}

	// Cleanup: ensure server is shutdown
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cleanupCancel()
	_ = server.Shutdown(cleanupCtx)
}

// Phase 4: /healthz Endpoint Method Restriction Testing

func Test_handleHealthz_GetRequest_Returns200(t *testing.T) {
	server := httpserver.New()

	// Start server
	go func() {
		_ = server.ListenAndServe()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Make GET request to /api/healthz
	req, err := http.NewRequest("GET", "http://localhost:8080/api/healthz", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Expected successful request, got error: %v", err)
	}
	defer resp.Body.Close()

	// Verify status code is 200
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	// Cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func Test_handleHealthz_PostRequest_Returns405(t *testing.T) {
	server := httpserver.New()

	// Start server
	go func() {
		_ = server.ListenAndServe()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Make POST request to /api/healthz
	req, err := http.NewRequest("POST", "http://localhost:8080/api/healthz", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Expected successful request, got error: %v", err)
	}
	defer resp.Body.Close()

	// Verify status code is 405
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status code %d, got %d", http.StatusMethodNotAllowed, resp.StatusCode)
	}

	// Verify Allow header is set to GET
	allowHeader := resp.Header.Get("Allow")
	if allowHeader != "GET" {
		t.Errorf("Expected Allow header to be 'GET', got '%s'", allowHeader)
	}

	// Cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func Test_handleHealthz_DeleteRequest_Returns405(t *testing.T) {
	server := httpserver.New()

	// Start server
	go func() {
		_ = server.ListenAndServe()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Make DELETE request to /api/healthz
	req, err := http.NewRequest("DELETE", "http://localhost:8080/api/healthz", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Expected successful request, got error: %v", err)
	}
	defer resp.Body.Close()

	// Verify status code is 405
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status code %d, got %d", http.StatusMethodNotAllowed, resp.StatusCode)
	}

	// Verify Allow header is set to GET
	allowHeader := resp.Header.Get("Allow")
	if allowHeader != "GET" {
		t.Errorf("Expected Allow header to be 'GET', got '%s'", allowHeader)
	}

	// Cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

// Phase 1: Chirp Validation Handler Testing

func Test_handleValidateChirp_ValidChirp_Returns200(t *testing.T) {
	reqBody := `{"body":"This is a valid chirp"}`
	req := httptest.NewRequest(http.MethodPost, "/api/validate_chirp", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()

	httpserver.HandleValidateChirp(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var response struct {
		CleanedBody string `json:"cleaned_body"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	expectedBody := "This is a valid chirp"
	if response.CleanedBody != expectedBody {
		t.Errorf("CleanedBody = %q, want %q", response.CleanedBody, expectedBody)
	}
}

func Test_handleValidateChirp_Exactly140Chars_Returns200(t *testing.T) {
	// Create exactly 140 character string
	chirp := strings.Repeat("a", 140)
	reqBody := `{"body":"` + chirp + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/validate_chirp", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()

	httpserver.HandleValidateChirp(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var response struct {
		CleanedBody string `json:"cleaned_body"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.CleanedBody != chirp {
		t.Errorf("CleanedBody length = %d, want %d", len(response.CleanedBody), len(chirp))
	}
}

func Test_handleValidateChirp_TooLong_Returns400(t *testing.T) {
	// Create 141 character string
	chirp := strings.Repeat("a", 141)
	reqBody := `{"body":"` + chirp + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/validate_chirp", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()

	httpserver.HandleValidateChirp(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var response struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	expectedError := "Chirp is too long"
	if response.Error != expectedError {
		t.Errorf("Error = %q, want %q", response.Error, expectedError)
	}
}

// Phase 2: Edge Case Tests

func Test_handleValidateChirp_MalformedJSON_Returns400(t *testing.T) {
	reqBody := `{invalid json`
	req := httptest.NewRequest(http.MethodPost, "/api/validate_chirp", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()

	httpserver.HandleValidateChirp(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var response struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	if response.Error == "" {
		t.Error("Expected error message in response")
	}
}

func Test_handleValidateChirp_EmptyBody_Returns400(t *testing.T) {
	reqBody := ``
	req := httptest.NewRequest(http.MethodPost, "/api/validate_chirp", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()

	httpserver.HandleValidateChirp(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var response struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	if response.Error == "" {
		t.Error("Expected error message in response")
	}
}

func Test_handleValidateChirp_MissingBodyField_ReturnsValid(t *testing.T) {
	reqBody := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/validate_chirp", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()

	httpserver.HandleValidateChirp(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var response struct {
		CleanedBody string `json:"cleaned_body"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.CleanedBody != "" {
		t.Errorf("CleanedBody = %q, want empty string", response.CleanedBody)
	}
}

// Phase 3: Method Restriction Tests for /api/validate_chirp

func Test_handleValidateChirp_GetRequest_Returns405(t *testing.T) {
	server := httpserver.New()

	// Start server
	go func() {
		_ = server.ListenAndServe()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Make GET request to /api/validate_chirp
	req, err := http.NewRequest("GET", "http://localhost:8080/api/validate_chirp", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Expected successful request, got error: %v", err)
	}
	defer resp.Body.Close()

	// Verify status code is 405
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status code %d, got %d", http.StatusMethodNotAllowed, resp.StatusCode)
	}

	// Verify Allow header is set to POST
	allowHeader := resp.Header.Get("Allow")
	if allowHeader != "POST" {
		t.Errorf("Expected Allow header to be 'POST', got '%s'", allowHeader)
	}

	// Cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func Test_handleValidateChirp_DeleteRequest_Returns405(t *testing.T) {
	server := httpserver.New()

	// Start server
	go func() {
		_ = server.ListenAndServe()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Make DELETE request to /api/validate_chirp
	req, err := http.NewRequest("DELETE", "http://localhost:8080/api/validate_chirp", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Expected successful request, got error: %v", err)
	}
	defer resp.Body.Close()

	// Verify status code is 405
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status code %d, got %d", http.StatusMethodNotAllowed, resp.StatusCode)
	}

	// Verify Allow header is set to POST
	allowHeader := resp.Header.Get("Allow")
	if allowHeader != "POST" {
		t.Errorf("Expected Allow header to be 'POST', got '%s'", allowHeader)
	}

	// Cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

// Phase 1: Profanity Filter Function Tests

func Test_cleanProfanity_NoMatches_ReturnsOriginal(t *testing.T) {
	input := "This is a nice clean message"
	result := httpserver.CleanProfanityForTest(input)

	if result != input {
		t.Errorf("Expected %q, got %q", input, result)
	}
}

func Test_cleanProfanity_SingleMatch_ReplacesWord(t *testing.T) {
	input := "What a kerfuffle this is"
	expected := "What a **** this is"
	result := httpserver.CleanProfanityForTest(input)

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func Test_cleanProfanity_CaseInsensitive_ReplacesAllCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase",
			input:    "I love kerfuffle",
			expected: "I love ****",
		},
		{
			name:     "uppercase",
			input:    "I love KERFUFFLE",
			expected: "I love ****",
		},
		{
			name:     "title case",
			input:    "I love Kerfuffle",
			expected: "I love ****",
		},
		{
			name:     "mixed case",
			input:    "I love KeRfUfFlE",
			expected: "I love ****",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := httpserver.CleanProfanityForTest(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func Test_cleanProfanity_AllThreeWords_ReplacesAll(t *testing.T) {
	input := "kerfuffle and sharbert and fornax"
	expected := "**** and **** and ****"
	result := httpserver.CleanProfanityForTest(input)

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func Test_cleanProfanity_MultipleInstances_ReplacesAll(t *testing.T) {
	input := "kerfuffle kerfuffle kerfuffle"
	expected := "**** **** ****"
	result := httpserver.CleanProfanityForTest(input)

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func Test_cleanProfanity_WithPunctuation_DoesNotReplace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "exclamation mark",
			input:    "Sharbert!",
			expected: "Sharbert!",
		},
		{
			name:     "period",
			input:    "kerfuffle.",
			expected: "kerfuffle.",
		},
		{
			name:     "comma",
			input:    "fornax,",
			expected: "fornax,",
		},
		{
			name:     "question mark",
			input:    "kerfuffle?",
			expected: "kerfuffle?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := httpserver.CleanProfanityForTest(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func Test_cleanProfanity_WithinWord_DoesNotReplace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "kerfuffle embedded",
			input:    "kerfuffled",
			expected: "kerfuffled",
		},
		{
			name:     "sharbert embedded",
			input:    "sharbertson",
			expected: "sharbertson",
		},
		{
			name:     "fornax embedded",
			input:    "fornaxation",
			expected: "fornaxation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := httpserver.CleanProfanityForTest(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}
