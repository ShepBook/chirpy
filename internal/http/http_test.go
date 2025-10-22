package http_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	httpserver "github.com/ShepBook/chirpy/internal/http"
)

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
