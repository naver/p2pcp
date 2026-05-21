// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package peers

import (
	"context"
	"testing"
)

func TestNewStaticProvider(t *testing.T) {
	tests := []struct {
		name        string
		sources     string
		defaultPort int
		wantPeers   []string
		wantErr     bool
	}{
		{
			name:        "single host",
			sources:     "host1",
			defaultPort: 8080,
			wantPeers:   []string{"host1:8080"},
			wantErr:     false,
		},
		{
			name:        "multiple hosts",
			sources:     "host1,host2,host3",
			defaultPort: 8080,
			wantPeers:   []string{"host1:8080", "host2:8080", "host3:8080"},
			wantErr:     false,
		},
		{
			name:        "hosts with ports",
			sources:     "host1:9090,host2:9091",
			defaultPort: 8080,
			wantPeers:   []string{"host1:9090", "host2:9091"},
			wantErr:     false,
		},
		{
			name:        "mixed hosts and IPs",
			sources:     "host1,192.168.1.1,host2:9090",
			defaultPort: 8080,
			wantPeers:   []string{"host1:8080", "192.168.1.1:8080", "host2:9090"},
			wantErr:     false,
		},
		{
			name:        "duplicate hosts",
			sources:     "host1,host1,host2",
			defaultPort: 8080,
			wantPeers:   []string{"host1:8080", "host2:8080"},
			wantErr:     false,
		},
		{
			name:        "IPv6 hosts",
			sources:     "[::1],[2001:db8::1]:9090",
			defaultPort: 8080,
			wantPeers:   []string{"[::1]:8080", "[2001:db8::1]:9090"},
			wantErr:     false,
		},
		{
			name:        "empty source in list",
			sources:     "host1,,host2",
			defaultPort: 8080,
			wantPeers:   nil,
			wantErr:     true,
		},
		{
			name:        "empty host with port",
			sources:     ":8080",
			defaultPort: 8080,
			wantPeers:   nil,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewStaticProvider(tt.sources, tt.defaultPort)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewStaticProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			peers, err := provider.GetPeers(context.Background())
			if err != nil {
				t.Fatalf("GetPeers() error = %v", err)
			}

			if len(peers) != len(tt.wantPeers) {
				t.Fatalf("Expected %d peers, got %d", len(tt.wantPeers), len(peers))
			}

			// Verify each expected peer exists in the result
			peerSet := make(map[string]bool)
			for _, p := range peers {
				peerSet[p] = true
			}
			for _, want := range tt.wantPeers {
				if !peerSet[want] {
					t.Errorf("Expected peer %q not found in result %v", want, peers)
				}
			}
		})
	}
}

func TestStaticProvider_AddStaticPeers(t *testing.T) {
	provider, err := NewStaticProvider("host1", 8080)
	if err != nil {
		t.Fatalf("NewStaticProvider failed: %v", err)
	}

	initialPeers, _ := provider.GetPeers(context.Background())
	if len(initialPeers) != 1 {
		t.Fatalf("Expected 1 initial peer, got %d", len(initialPeers))
	}
	if initialPeers[0] != "host1:8080" {
		t.Fatalf("Expected initial peer 'host1:8080', got %q", initialPeers[0])
	}

	// Add more peers
	provider.AddStaticPeers([]string{"host2:9090", "host3:9091"})

	allPeers, _ := provider.GetPeers(context.Background())
	if len(allPeers) != 3 {
		t.Fatalf("Expected 3 peers after adding, got %d", len(allPeers))
	}

	// Verify all expected peers exist
	expectedPeers := map[string]bool{
		"host1:8080": true,
		"host2:9090": true,
		"host3:9091": true,
	}
	for _, peer := range allPeers {
		if !expectedPeers[peer] {
			t.Errorf("Unexpected peer %q in result", peer)
		}
		delete(expectedPeers, peer)
	}
	if len(expectedPeers) > 0 {
		t.Errorf("Missing peers: %v", expectedPeers)
	}

	// Add duplicate
	provider.AddStaticPeers([]string{"host2:9090"})
	allPeers, _ = provider.GetPeers(context.Background())
	if len(allPeers) != 3 {
		t.Errorf("Expected 3 peers after adding duplicate, got %d", len(allPeers))
	}
}

func TestStaticProvider_GetPeers(t *testing.T) {
	provider, err := NewStaticProvider("host1,host2", 8080)
	if err != nil {
		t.Fatalf("NewStaticProvider failed: %v", err)
	}

	// Test that GetPeers returns correct and consistent results
	peers1, err := provider.GetPeers(context.Background())
	if err != nil {
		t.Fatalf("GetPeers() error = %v", err)
	}

	// Verify actual peer values
	expectedPeers := map[string]bool{
		"host1:8080": true,
		"host2:8080": true,
	}
	if len(peers1) != len(expectedPeers) {
		t.Fatalf("Expected %d peers, got %d", len(expectedPeers), len(peers1))
	}
	for _, peer := range peers1 {
		if !expectedPeers[peer] {
			t.Errorf("Unexpected peer %q, expected one of %v", peer, expectedPeers)
		}
	}

	// Test consistency
	peers2, err := provider.GetPeers(context.Background())
	if err != nil {
		t.Fatalf("GetPeers() error = %v", err)
	}

	if len(peers1) != len(peers2) {
		t.Fatalf("GetPeers() returned different lengths: %d vs %d", len(peers1), len(peers2))
	}
}
