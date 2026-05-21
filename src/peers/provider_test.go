// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package peers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNormalizePeerAddress(t *testing.T) {
	const DefaultPort = 10090

	tests := []struct {
		name        string
		input       string
		want        string
		wantErr     bool
		errContains string
	}{
		// Success cases
		{
			name:  "host",
			input: "MyHost",
			want:  fmt.Sprint("MyHost:", DefaultPort),
		},
		{
			name:  "host port",
			input: "MyHost:1234",
			want:  "MyHost:1234",
		},
		{
			name:  "ipv4",
			input: "127.0.10.100",
			want:  fmt.Sprint("127.0.10.100:", DefaultPort),
		},
		{
			name:  "ipv4 port",
			input: "127.0.10.100:1234",
			want:  "127.0.10.100:1234",
		},
		{
			name:  "ipv6 without brackets",
			input: "FF01:0:0:0:0:0:0:2",
			want:  fmt.Sprint("[FF01:0:0:0:0:0:0:2]:", DefaultPort),
		},
		{
			name:  "ipv6 with brackets",
			input: "[FF01:0:0:0:0:0:0:2]",
			want:  fmt.Sprint("[FF01:0:0:0:0:0:0:2]:", DefaultPort),
		},
		{
			name:  "ipv6 port",
			input: "[FF01:0:0:0:0:0:0:2]:1234",
			want:  "[FF01:0:0:0:0:0:0:2]:1234",
		},
		{
			name:  "url with trailing slash is trimmed",
			input: "localhost:1234/",
			want:  "localhost:1234",
		},
		{
			name:  "url with path",
			input: "localhost/test",
			want:  "localhost/test",
		},
		{
			name:  "url with port and path",
			input: "localhost:1234/test",
			want:  "localhost:1234/test",
		},
		{
			name:  "http scheme is stripped",
			input: "http://example.com:8080",
			want:  "example.com:8080",
		},
		{
			name:  "https scheme is stripped",
			input: "https://example.com",
			want:  fmt.Sprint("example.com:", DefaultPort),
		},
		{
			name:  "http scheme with path",
			input: "http://example.com:8080/api/peers",
			want:  "example.com:8080/api/peers",
		},
		{
			name:  "whitespace is trimmed",
			input: "  host1:8080  ",
			want:  "host1:8080",
		},
		// Error cases
		{
			name:        "empty string",
			input:       "",
			wantErr:     true,
			errContains: "empty peer address",
		},
		{
			name:        "whitespace only",
			input:       "   ",
			wantErr:     true,
			errContains: "empty peer address",
		},
		{
			name:        "empty host with port",
			input:       ":8080",
			wantErr:     true,
			errContains: "empty host",
		},
		{
			name:        "http scheme with empty host",
			input:       "http:///path",
			wantErr:     true,
			errContains: "empty host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizePeerAddress(tt.input, DefaultPort)
			if (err != nil) != tt.wantErr {
				t.Errorf("normalizePeerAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("normalizePeerAddress() error = %q, want error containing %q", err.Error(), tt.errContains)
				}
				return
			}
			if got != tt.want {
				t.Errorf("normalizePeerAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeduplicatePeers(t *testing.T) {
	tests := []struct {
		name      string
		input     []string
		wantCount int
		wantPeers map[string]bool // expected unique peers
	}{
		{
			name:      "no duplicates",
			input:     []string{"a", "b", "c"},
			wantCount: 3,
			wantPeers: map[string]bool{"a": true, "b": true, "c": true},
		},
		{
			name:      "with duplicates",
			input:     []string{"a", "b", "a", "c", "b"},
			wantCount: 3,
			wantPeers: map[string]bool{"a": true, "b": true, "c": true},
		},
		{
			name:      "all same",
			input:     []string{"a", "a", "a"},
			wantCount: 1,
			wantPeers: map[string]bool{"a": true},
		},
		{
			name:      "empty",
			input:     []string{},
			wantCount: 0,
			wantPeers: map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deduplicatePeers(tt.input)

			// Check count
			if len(got) != tt.wantCount {
				t.Errorf("deduplicatePeers() returned %d items, want %d", len(got), tt.wantCount)
			}

			// Check no duplicates in result
			seen := make(map[string]bool)
			for _, peer := range got {
				if seen[peer] {
					t.Errorf("deduplicatePeers() returned duplicate: %q", peer)
				}
				seen[peer] = true
			}

			// Check all expected peers are present
			for peer := range tt.wantPeers {
				if !seen[peer] {
					t.Errorf("deduplicatePeers() missing expected peer: %q", peer)
				}
			}

			// Check no unexpected peers
			for peer := range seen {
				if !tt.wantPeers[peer] {
					t.Errorf("deduplicatePeers() returned unexpected peer: %q", peer)
				}
			}
		})
	}
}

func TestNewFactory(t *testing.T) {
	tests := []struct {
		name        string
		staticPeers string
		defaultPort int
		wantErr     bool
	}{
		{
			name:        "no static peers",
			staticPeers: "",
			defaultPort: 8080,
			wantErr:     false,
		},
		{
			name:        "single static peer",
			staticPeers: "host1:9090",
			defaultPort: 8080,
			wantErr:     false,
		},
		{
			name:        "multiple static peers",
			staticPeers: "host1:9090,host2:9091",
			defaultPort: 8080,
			wantErr:     false,
		},
		{
			name:        "static peer without port uses default",
			staticPeers: "host1",
			defaultPort: 8080,
			wantErr:     false,
		},
		{
			name:        "invalid static peer",
			staticPeers: ",",
			defaultPort: 8080,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, err := NewFactory(tt.staticPeers, tt.defaultPort, 5*time.Second, 2*time.Second)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewFactory() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && factory == nil {
				t.Error("Expected factory to be non-nil")
			}
		})
	}
}

func TestFactory_Create(t *testing.T) {
	t.Run("returns static provider when no options", func(t *testing.T) {
		factory, err := NewFactory("host1:8080", 8080, 5*time.Second, 2*time.Second)
		if err != nil {
			t.Fatalf("NewFactory failed: %v", err)
		}

		provider, err := factory.Create()
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		peers, err := provider.GetPeers(context.Background())
		if err != nil {
			t.Fatalf("GetPeers failed: %v", err)
		}

		if len(peers) != 1 || peers[0] != "host1:8080" {
			t.Errorf("Expected ['host1:8080'], got %v", peers)
		}
	})

	t.Run("returns nil when no static peers and no options", func(t *testing.T) {
		factory, err := NewFactory("", 8080, 5*time.Second, 2*time.Second)
		if err != nil {
			t.Fatalf("NewFactory failed: %v", err)
		}

		provider, err := factory.Create()
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if provider != nil {
			t.Error("Expected nil provider when no static peers and no options")
		}
	})

	t.Run("creates URL provider", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("dynamic1:9090"))
		}))
		defer server.Close()

		factory, err := NewFactory("", 8080, 5*time.Second, 2*time.Second)
		if err != nil {
			t.Fatalf("NewFactory failed: %v", err)
		}

		provider, err := factory.Create(WithURL(server.URL))
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		peers, err := provider.GetPeers(context.Background())
		if err != nil {
			t.Fatalf("GetPeers failed: %v", err)
		}

		if len(peers) != 1 {
			t.Errorf("Expected 1 peer, got %d", len(peers))
		}
	})

	t.Run("URL provider includes static peers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("dynamic1:9090"))
		}))
		defer server.Close()

		factory, err := NewFactory("static1:8080", 8080, 5*time.Second, 2*time.Second)
		if err != nil {
			t.Fatalf("NewFactory failed: %v", err)
		}

		provider, err := factory.Create(WithURL(server.URL))
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		peers, err := provider.GetPeers(context.Background())
		if err != nil {
			t.Fatalf("GetPeers failed: %v", err)
		}

		if len(peers) != 2 {
			t.Errorf("Expected 2 peers (1 dynamic + 1 static), got %d", len(peers))
		}
	})

	t.Run("creates A record provider", func(t *testing.T) {
		factory, err := NewFactory("", 8080, 5*time.Second, 2*time.Second)
		if err != nil {
			t.Fatalf("NewFactory failed: %v", err)
		}

		provider, err := factory.Create(WithARecord("example.com"))
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if provider == nil {
			t.Error("Expected non-nil provider")
		}
	})

	t.Run("creates SRV record provider", func(t *testing.T) {
		factory, err := NewFactory("", 8080, 5*time.Second, 2*time.Second)
		if err != nil {
			t.Fatalf("NewFactory failed: %v", err)
		}

		provider, err := factory.Create(WithSRVRecord("_service._tcp.example.com"))
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if provider == nil {
			t.Error("Expected non-nil provider")
		}
	})

	t.Run("error when multiple options specified", func(t *testing.T) {
		factory, err := NewFactory("", 8080, 5*time.Second, 2*time.Second)
		if err != nil {
			t.Fatalf("NewFactory failed: %v", err)
		}

		_, err = factory.Create(WithURL("http://example.com"), WithARecord("example.com"))
		if err == nil {
			t.Error("Expected error when multiple options specified")
		}
	})

	t.Run("error with invalid URL", func(t *testing.T) {
		factory, err := NewFactory("", 8080, 5*time.Second, 2*time.Second)
		if err != nil {
			t.Fatalf("NewFactory failed: %v", err)
		}

		_, err = factory.Create(WithURL("not-a-valid-url"))
		if err == nil {
			t.Error("Expected error with invalid URL")
		}
	})

	t.Run("empty options returns static provider", func(t *testing.T) {
		factory, err := NewFactory("192.168.1.1:8080", 8080, 5*time.Second, 2*time.Second)
		if err != nil {
			t.Fatalf("NewFactory failed: %v", err)
		}

		provider, err := factory.Create(
			WithARecord(""),
			WithSRVRecord(""),
			WithURL(""),
		)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if provider == nil {
			t.Fatal("expected static provider, got nil")
		}

		peers, err := provider.GetPeers(context.Background())
		if err != nil {
			t.Fatalf("GetPeers failed: %v", err)
		}

		if len(peers) != 1 || peers[0] != "192.168.1.1:8080" {
			t.Errorf("expected static peers [192.168.1.1:8080], got %v", peers)
		}
	})

	t.Run("WithDNSServer option", func(t *testing.T) {
		factory, err := NewFactory("", 8080, 5*time.Second, 2*time.Second)
		if err != nil {
			t.Fatalf("NewFactory failed: %v", err)
		}

		provider, err := factory.Create(WithARecord("example.com"), WithDNSServer("8.8.8.8:53"))
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if provider == nil {
			t.Error("Expected non-nil provider")
		}
	})
}
