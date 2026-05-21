// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package peers

import (
	"context"
	"testing"
	"time"

	"github.com/naver/p2pcp/testutil"
)

func TestNewARecordProvider(t *testing.T) {
	tests := []struct {
		name        string
		fqdn        string
		timeout     time.Duration
		defaultPort int
		dnsServer   string
		wantErr     bool
		wantFqdn    string
		wantPort    int
	}{
		{
			name:        "valid FQDN",
			fqdn:        "example.com",
			timeout:     5 * time.Second,
			defaultPort: 8080,
			dnsServer:   "",
			wantErr:     false,
			wantFqdn:    "example.com",
			wantPort:    8080,
		},
		{
			name:        "valid FQDN with DNS server",
			fqdn:        "example.com",
			timeout:     5 * time.Second,
			defaultPort: 8080,
			dnsServer:   "127.0.0.1:5353",
			wantErr:     false,
			wantFqdn:    "example.com",
			wantPort:    8080,
		},
		{
			name:        "FQDN with whitespace is trimmed",
			fqdn:        "  example.com  ",
			timeout:     5 * time.Second,
			defaultPort: 9090,
			dnsServer:   "",
			wantErr:     false,
			wantFqdn:    "example.com",
			wantPort:    9090,
		},
		{
			name:        "empty FQDN",
			fqdn:        "",
			timeout:     5 * time.Second,
			defaultPort: 8080,
			dnsServer:   "",
			wantErr:     true,
		},
		{
			name:        "whitespace FQDN",
			fqdn:        "   ",
			timeout:     5 * time.Second,
			defaultPort: 8080,
			dnsServer:   "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewARecordProvider(tt.fqdn, tt.timeout, tt.defaultPort, tt.dnsServer)
			if tt.wantErr {
				if err == nil {
					t.Error("NewARecordProvider() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewARecordProvider() unexpected error: %v", err)
			}
			if provider.fqdn != tt.wantFqdn {
				t.Errorf("fqdn = %q, want %q", provider.fqdn, tt.wantFqdn)
			}
			if provider.defaultPort != tt.wantPort {
				t.Errorf("defaultPort = %d, want %d", provider.defaultPort, tt.wantPort)
			}
		})
	}
}

func TestARecordProvider_GetPeers(t *testing.T) {
	// Start DNS mock server
	dnsServer := testutil.NewDNSMockServer("127.0.0.1:15353")
	dnsServer.AddRecord("test.example.com", []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"})

	go func() {
		if err := dnsServer.Start(); err != nil {
			t.Logf("DNS server error: %v", err)
		}
	}()
	defer dnsServer.Stop()

	// Give DNS server time to start
	time.Sleep(100 * time.Millisecond)

	t.Run("successful A record lookup", func(t *testing.T) {
		provider, err := NewARecordProvider("test.example.com", 2*time.Second, 8080, "127.0.0.1:15353")
		if err != nil {
			t.Fatalf("NewARecordProvider failed: %v", err)
		}

		peers, err := provider.GetPeers(context.Background())
		if err != nil {
			t.Fatalf("GetPeers failed: %v", err)
		}

		if len(peers) != 3 {
			t.Errorf("Expected 3 peers, got %d", len(peers))
		}

		// Verify peers have correct format (IP:port)
		expectedPeers := map[string]bool{
			"192.168.1.1:8080": true,
			"192.168.1.2:8080": true,
			"192.168.1.3:8080": true,
		}
		for _, peer := range peers {
			if !expectedPeers[peer] {
				t.Errorf("Unexpected peer: %s", peer)
			}
		}
	})

	t.Run("combines static and DNS peers", func(t *testing.T) {
		provider, err := NewARecordProvider("test.example.com", 2*time.Second, 8080, "127.0.0.1:15353")
		if err != nil {
			t.Fatalf("NewARecordProvider failed: %v", err)
		}

		staticPeers := []string{"static1:9090"}
		provider.AddStaticPeers(staticPeers)

		peers, err := provider.GetPeers(context.Background())
		if err != nil {
			t.Fatalf("GetPeers failed: %v", err)
		}

		expectedPeers := map[string]bool{
			"192.168.1.1:8080": true,
			"192.168.1.2:8080": true,
			"192.168.1.3:8080": true,
			"static1:9090":     true,
		}
		if len(peers) != len(expectedPeers) {
			t.Errorf("Expected %d peers, got %d", len(expectedPeers), len(peers))
		}
		for _, peer := range peers {
			if !expectedPeers[peer] {
				t.Errorf("Unexpected peer: %s", peer)
			}
		}
	})

	t.Run("deduplicates peers", func(t *testing.T) {
		provider, err := NewARecordProvider("test.example.com", 2*time.Second, 8080, "127.0.0.1:15353")
		if err != nil {
			t.Fatalf("NewARecordProvider failed: %v", err)
		}

		// Add static peer that's same as DNS result
		provider.AddStaticPeers([]string{"192.168.1.1:8080"})

		peers, err := provider.GetPeers(context.Background())
		if err != nil {
			t.Fatalf("GetPeers failed: %v", err)
		}

		// Should still be 3 unique peers
		if len(peers) != 3 {
			t.Errorf("Expected 3 unique peers after deduplication, got %d", len(peers))
		}
	})
}

func TestARecordProvider_AddStaticPeers(t *testing.T) {
	provider, err := NewARecordProvider("example.com", 5*time.Second, 8080, "")
	if err != nil {
		t.Fatalf("NewARecordProvider failed: %v", err)
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

func TestNewSRVRecordProvider(t *testing.T) {
	tests := []struct {
		name      string
		fqdn      string
		timeout   time.Duration
		dnsServer string
		wantErr   bool
	}{
		{
			name:      "valid FQDN",
			fqdn:      "_service._tcp.example.com",
			timeout:   5 * time.Second,
			dnsServer: "",
			wantErr:   false,
		},
		{
			name:      "valid FQDN with DNS server",
			fqdn:      "_service._tcp.example.com",
			timeout:   5 * time.Second,
			dnsServer: "127.0.0.1:5353",
			wantErr:   false,
		},
		{
			name:      "empty FQDN",
			fqdn:      "",
			timeout:   5 * time.Second,
			dnsServer: "",
			wantErr:   true,
		},
		{
			name:      "whitespace FQDN",
			fqdn:      "   ",
			timeout:   5 * time.Second,
			dnsServer: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewSRVRecordProvider(tt.fqdn, tt.timeout, tt.dnsServer)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewSRVRecordProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && provider == nil {
				t.Error("Expected provider to be non-nil")
			}
		})
	}
}

func TestSRVRecordProvider_GetPeers(t *testing.T) {
	// Start DNS mock server
	dnsServer := testutil.NewDNSMockServer("127.0.0.1:15354")

	// Add SRV records
	dnsServer.AddSRVRecord("_p2pcp._tcp.example.com", []testutil.SRVRecord{
		{Priority: 10, Weight: 10, Port: 9090, Target: "192.168.1.10"},
		{Priority: 10, Weight: 10, Port: 9091, Target: "192.168.1.11"},
	})

	// Add A records for SRV targets
	dnsServer.AddRecord("192.168.1.10.local", []string{"192.168.1.10"})
	dnsServer.AddRecord("192.168.1.11.local", []string{"192.168.1.11"})

	go func() {
		if err := dnsServer.Start(); err != nil {
			t.Logf("DNS server error: %v", err)
		}
	}()
	defer dnsServer.Stop()

	// Give DNS server time to start
	time.Sleep(100 * time.Millisecond)

	t.Run("successful SRV record lookup", func(t *testing.T) {
		provider, err := NewSRVRecordProvider("_p2pcp._tcp.example.com", 2*time.Second, "127.0.0.1:15354")
		if err != nil {
			t.Fatalf("NewSRVRecordProvider failed: %v", err)
		}

		peers, err := provider.GetPeers(context.Background())
		if err != nil {
			t.Fatalf("GetPeers failed: %v", err)
		}

		if len(peers) != 2 {
			t.Errorf("Expected 2 peers, got %d", len(peers))
		}

		// Verify peers have correct port from SRV record
		expectedPeers := map[string]bool{
			"192.168.1.10:9090": true,
			"192.168.1.11:9091": true,
		}
		for _, peer := range peers {
			if !expectedPeers[peer] {
				t.Errorf("Unexpected peer: %s", peer)
			}
		}
	})

	t.Run("combines static and SRV peers", func(t *testing.T) {
		provider, err := NewSRVRecordProvider("_p2pcp._tcp.example.com", 2*time.Second, "127.0.0.1:15354")
		if err != nil {
			t.Fatalf("NewSRVRecordProvider failed: %v", err)
		}

		staticPeers := []string{"static1:8080"}
		provider.AddStaticPeers(staticPeers)

		peers, err := provider.GetPeers(context.Background())
		if err != nil {
			t.Fatalf("GetPeers failed: %v", err)
		}

		expectedPeers := map[string]bool{
			"192.168.1.10:9090": true,
			"192.168.1.11:9091": true,
			"static1:8080":      true,
		}
		if len(peers) != len(expectedPeers) {
			t.Errorf("Expected %d peers, got %d", len(expectedPeers), len(peers))
		}
		for _, peer := range peers {
			if !expectedPeers[peer] {
				t.Errorf("Unexpected peer: %s", peer)
			}
		}
	})
}

func TestSRVRecordProvider_AddStaticPeers(t *testing.T) {
	provider, err := NewSRVRecordProvider("_service._tcp.example.com", 5*time.Second, "")
	if err != nil {
		t.Fatalf("NewSRVRecordProvider failed: %v", err)
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
