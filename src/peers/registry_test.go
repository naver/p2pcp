// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package peers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewRegistry(t *testing.T) {
	tests := []struct {
		name              string
		heartbeatURL      string
		address           string
		uuid              string
		heartbeatInterval time.Duration
		timeout           time.Duration
		wantErr           bool
		errContains       string
	}{
		{
			name:              "valid configuration",
			heartbeatURL:      "http://example.com/heartbeat",
			address:           "192.168.1.1:8080",
			uuid:              "test-uuid-123",
			heartbeatInterval: 10 * time.Second,
			timeout:           5 * time.Second,
			wantErr:           false,
		},
		{
			name:              "empty heartbeat URL",
			heartbeatURL:      "",
			address:           "192.168.1.1:8080",
			uuid:              "test-uuid-123",
			heartbeatInterval: 10 * time.Second,
			timeout:           5 * time.Second,
			wantErr:           true,
			errContains:       "-peer-registry-heartbeat-url",
		},
		{
			name:              "empty address",
			heartbeatURL:      "http://example.com/heartbeat",
			address:           "",
			uuid:              "test-uuid-123",
			heartbeatInterval: 10 * time.Second,
			timeout:           5 * time.Second,
			wantErr:           true,
			errContains:       "-peer-registry-self-endpoint",
		},
		{
			name:              "empty uuid",
			heartbeatURL:      "http://example.com/heartbeat",
			address:           "192.168.1.1:8080",
			uuid:              "",
			heartbeatInterval: 10 * time.Second,
			timeout:           5 * time.Second,
			wantErr:           true,
			errContains:       "uuid",
		},
		{
			name:              "zero heartbeat interval",
			heartbeatURL:      "http://example.com/heartbeat",
			address:           "192.168.1.1:8080",
			uuid:              "test-uuid-123",
			heartbeatInterval: 0,
			timeout:           5 * time.Second,
			wantErr:           true,
			errContains:       "-peer-registry-heartbeat-interval",
		},
		{
			name:              "negative heartbeat interval",
			heartbeatURL:      "http://example.com/heartbeat",
			address:           "192.168.1.1:8080",
			uuid:              "test-uuid-123",
			heartbeatInterval: -1 * time.Second,
			timeout:           5 * time.Second,
			wantErr:           true,
			errContains:       "-peer-registry-heartbeat-interval",
		},
		{
			name:              "zero timeout",
			heartbeatURL:      "http://example.com/heartbeat",
			address:           "192.168.1.1:8080",
			uuid:              "test-uuid-123",
			heartbeatInterval: 10 * time.Second,
			timeout:           0,
			wantErr:           true,
			errContains:       "-peer-registry-timeout",
		},
		{
			name:              "negative timeout",
			heartbeatURL:      "http://example.com/heartbeat",
			address:           "192.168.1.1:8080",
			uuid:              "test-uuid-123",
			heartbeatInterval: 10 * time.Second,
			timeout:           -1 * time.Second,
			wantErr:           true,
			errContains:       "-peer-registry-timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry, err := NewRegistry(tt.heartbeatURL, tt.address, tt.uuid, tt.heartbeatInterval, tt.timeout)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRegistry() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && registry == nil {
				t.Error("Expected registry to be non-nil")
			}
		})
	}
}

func TestRegistry_StartStop(t *testing.T) {
	var heartbeatCount atomic.Int32
	heartbeatReached := make(chan struct{})
	const targetHeartbeats = int32(3)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		body, _ := io.ReadAll(r.Body)
		var req HeartbeatRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("Failed to unmarshal request: %v", err)
		}

		if req.UUID != "test-uuid" {
			t.Errorf("Expected UUID 'test-uuid', got '%s'", req.UUID)
		}
		if req.Address != "192.168.1.1:8080" {
			t.Errorf("Expected Address '192.168.1.1:8080', got '%s'", req.Address)
		}

		count := heartbeatCount.Add(1)
		if count == targetHeartbeats {
			close(heartbeatReached)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// heartbeatInterval must be at least 1 second (converted to int seconds internally)
	registry, err := NewRegistry(server.URL, "192.168.1.1:8080", "test-uuid", 1*time.Second, 1*time.Second)
	if err != nil {
		t.Fatalf("NewRegistry failed: %v", err)
	}

	// Start registry - this sends initial heartbeat
	if err := registry.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for heartbeats with timeout
	select {
	case <-heartbeatReached:
		// Success - required heartbeats received
	case <-time.After(5 * time.Second):
		t.Errorf("Timeout waiting for %d heartbeats, got %d", targetHeartbeats, heartbeatCount.Load())
	}

	if heartbeatCount.Load() < targetHeartbeats {
		t.Errorf("Expected at least %d heartbeats, got %d", targetHeartbeats, heartbeatCount.Load())
	}

	// Stop registry
	registry.Stop()
}

func TestRegistry_StartAlreadyRunning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	registry, err := NewRegistry(server.URL, "192.168.1.1:8080", "test-uuid", 1*time.Second, 1*time.Second)
	if err != nil {
		t.Fatalf("NewRegistry failed: %v", err)
	}

	if err := registry.Start(); err != nil {
		t.Fatalf("First Start failed: %v", err)
	}
	defer registry.Stop()

	// Try to start again
	err = registry.Start()
	if err == nil {
		t.Error("Expected error when starting already running registry")
	}
}

func TestRegistry_StopNotRunning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	registry, err := NewRegistry(server.URL, "192.168.1.1:8080", "test-uuid", 1*time.Second, 1*time.Second)
	if err != nil {
		t.Fatalf("NewRegistry failed: %v", err)
	}

	// Stop without starting - should not panic
	registry.Stop()
}

func TestRegistry_HeartbeatFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	registry, err := NewRegistry(server.URL, "192.168.1.1:8080", "test-uuid", 1*time.Second, 1*time.Second)
	if err != nil {
		t.Fatalf("NewRegistry failed: %v", err)
	}

	// Start should fail because initial heartbeat fails
	err = registry.Start()
	if err == nil {
		t.Error("Expected error when initial heartbeat fails")
		registry.Stop()
	}
}

func TestRegistry_HeartbeatExpiresInSeconds(t *testing.T) {
	var receivedExpires int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req HeartbeatRequest
		json.Unmarshal(body, &req)
		receivedExpires = req.ExpiresInSeconds
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	heartbeatInterval := 10 * time.Second
	registry, err := NewRegistry(server.URL, "192.168.1.1:8080", "test-uuid", heartbeatInterval, 1*time.Second)
	if err != nil {
		t.Fatalf("NewRegistry failed: %v", err)
	}

	if err := registry.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	registry.Stop()

	// ExpiresInSeconds should be 2x heartbeat interval
	expectedExpires := int(heartbeatInterval.Seconds()) * 2
	if receivedExpires != expectedExpires {
		t.Errorf("Expected ExpiresInSeconds %d, got %d", expectedExpires, receivedExpires)
	}
}
