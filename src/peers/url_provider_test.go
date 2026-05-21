// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package peers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewURLProvider(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		defaultPort int
		wantErr     bool
	}{
		{
			name:        "valid http URL",
			url:         "http://example.com/peers",
			defaultPort: 8080,
			wantErr:     false,
		},
		{
			name:        "valid https URL",
			url:         "https://example.com/peers",
			defaultPort: 8080,
			wantErr:     false,
		},
		{
			name:        "empty URL",
			url:         "",
			defaultPort: 8080,
			wantErr:     true,
		},
		{
			name:        "invalid URL without scheme",
			url:         "example.com/peers",
			defaultPort: 8080,
			wantErr:     true,
		},
		{
			name:        "invalid URL with ftp scheme",
			url:         "ftp://example.com/peers",
			defaultPort: 8080,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewURLProvider(tt.url, tt.defaultPort)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewURLProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && provider == nil {
				t.Error("Expected provider to be non-nil")
			}
		})
	}
}

func TestURLProvider_GetPeers(t *testing.T) {
	t.Run("successful peer list retrieval", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("host1:9090,host2:9091,192.168.1.1"))
		}))
		defer server.Close()

		provider, err := NewURLProvider(server.URL, 8080)
		if err != nil {
			t.Fatalf("NewURLProvider failed: %v", err)
		}

		peers, err := provider.GetPeers(context.Background())
		if err != nil {
			t.Fatalf("GetPeers failed: %v", err)
		}

		expectedPeers := map[string]bool{
			"host1:9090":       true,
			"host2:9091":       true,
			"192.168.1.1:8080": true, // default port applied
		}
		if len(peers) != len(expectedPeers) {
			t.Errorf("Expected %d peers, got %d", len(expectedPeers), len(peers))
		}
		for _, peer := range peers {
			if !expectedPeers[peer] {
				t.Errorf("Unexpected peer %q, expected one of %v", peer, expectedPeers)
			}
		}
	})

	t.Run("empty response returns static peers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			// Empty response
		}))
		defer server.Close()

		provider, err := NewURLProvider(server.URL, 8080)
		if err != nil {
			t.Fatalf("NewURLProvider failed: %v", err)
		}

		staticPeers := []string{"static1:8080", "static2:8080"}
		provider.AddStaticPeers(staticPeers)

		peers, err := provider.GetPeers(context.Background())
		if err != nil {
			t.Fatalf("GetPeers failed: %v", err)
		}

		if len(peers) != len(staticPeers) {
			t.Errorf("Expected %d peers, got %d", len(staticPeers), len(peers))
		}

		// Verify static peers are returned correctly
		expectedPeers := map[string]bool{
			"static1:8080": true,
			"static2:8080": true,
		}
		for _, peer := range peers {
			if !expectedPeers[peer] {
				t.Errorf("Unexpected peer %q, expected one of %v", peer, expectedPeers)
			}
		}
	})

	t.Run("combines static and dynamic peers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("dynamic1:9090,dynamic2:9091"))
		}))
		defer server.Close()

		provider, err := NewURLProvider(server.URL, 8080)
		if err != nil {
			t.Fatalf("NewURLProvider failed: %v", err)
		}

		staticPeers := []string{"static1:8080"}
		provider.AddStaticPeers(staticPeers)

		peers, err := provider.GetPeers(context.Background())
		if err != nil {
			t.Fatalf("GetPeers failed: %v", err)
		}

		expectedPeers := map[string]bool{
			"dynamic1:9090": true,
			"dynamic2:9091": true,
			"static1:8080":  true,
		}
		if len(peers) != len(expectedPeers) {
			t.Errorf("Expected %d peers (2 dynamic + 1 static), got %d", len(expectedPeers), len(peers))
		}
		for _, peer := range peers {
			if !expectedPeers[peer] {
				t.Errorf("Unexpected peer %q, expected one of %v", peer, expectedPeers)
			}
		}
	})

	t.Run("deduplicates peers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("host1:9090,host2:9091,host1:9090"))
		}))
		defer server.Close()

		provider, err := NewURLProvider(server.URL, 8080)
		if err != nil {
			t.Fatalf("NewURLProvider failed: %v", err)
		}

		peers, err := provider.GetPeers(context.Background())
		if err != nil {
			t.Fatalf("GetPeers failed: %v", err)
		}

		expectedPeers := map[string]bool{
			"host1:9090": true,
			"host2:9091": true,
		}
		if len(peers) != len(expectedPeers) {
			t.Errorf("Expected %d unique peers, got %d", len(expectedPeers), len(peers))
		}
		for _, peer := range peers {
			if !expectedPeers[peer] {
				t.Errorf("Unexpected peer %q, expected one of %v", peer, expectedPeers)
			}
		}
	})

	t.Run("handles server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		provider, err := NewURLProvider(server.URL, 8080)
		if err != nil {
			t.Fatalf("NewURLProvider failed: %v", err)
		}

		_, err = provider.GetPeers(context.Background())
		if err == nil {
			t.Error("Expected error when server returns 500")
		}
	})

	t.Run("handles invalid peer address", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			// Empty string is invalid peer address
			w.Write([]byte("host1:9090,,host2:9091"))
		}))
		defer server.Close()

		provider, err := NewURLProvider(server.URL, 8080)
		if err != nil {
			t.Fatalf("NewURLProvider failed: %v", err)
		}

		_, err = provider.GetPeers(context.Background())
		if err == nil {
			t.Error("Expected error for invalid peer address")
		}
	})

	t.Run("adds default port to peers without port", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("host1,host2:9091"))
		}))
		defer server.Close()

		defaultPort := 8080
		provider, err := NewURLProvider(server.URL, defaultPort)
		if err != nil {
			t.Fatalf("NewURLProvider failed: %v", err)
		}

		peers, err := provider.GetPeers(context.Background())
		if err != nil {
			t.Fatalf("GetPeers failed: %v", err)
		}

		// Verify first peer got default port
		found := false
		for _, peer := range peers {
			if peer == "host1:8080" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected host1 to have default port 8080")
		}
	})
}

func TestURLProvider_AddStaticPeers(t *testing.T) {
	provider, err := NewURLProvider("http://example.com", 8080)
	if err != nil {
		t.Fatalf("NewURLProvider failed: %v", err)
	}

	provider.AddStaticPeers([]string{"peer1:9090"})
	provider.AddStaticPeers([]string{"peer2:9091"})

	if len(provider.peers) != 2 {
		t.Errorf("Expected 2 static peers, got %d", len(provider.peers))
	}

	// Add duplicate
	provider.AddStaticPeers([]string{"peer1:9090"})
	if len(provider.peers) != 2 {
		t.Errorf("Expected 2 peers after adding duplicate, got %d", len(provider.peers))
	}
}
