// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package peers

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mockProvider struct {
	peers    []string
	mu       sync.Mutex
	errCount atomic.Int32 // return error for first N calls
}

func (m *mockProvider) GetPeers(ctx context.Context) ([]string, error) {
	if m.errCount.Load() > 0 {
		m.errCount.Add(-1)
		return nil, errors.New("mock error")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.peers...), nil
}

func (m *mockProvider) AddStaticPeers(peers []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.peers = append(m.peers, peers...)
}

func (m *mockProvider) setPeers(peers []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.peers = append([]string{}, peers...)
}

func (m *mockProvider) setErrorCount(n int) {
	m.errCount.Store(int32(n))
}

// peerTracker tracks current peers and detects duplicate add/remove errors
type peerTracker struct {
	t            *testing.T
	currentPeers map[string]bool
	mu           sync.Mutex
	onChange     chan struct{} // signaled on any add/remove
}

func newPeerTracker(t *testing.T) *peerTracker {
	return &peerTracker{
		t:            t,
		currentPeers: make(map[string]bool),
		onChange:     make(chan struct{}, 100), // buffered to avoid blocking
	}
}

func (pt *peerTracker) onAdd(host string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if pt.currentPeers[host] {
		pt.t.Errorf("Duplicate add: peer %q already exists", host)
	}
	pt.currentPeers[host] = true
	select {
	case pt.onChange <- struct{}{}:
	default:
	}
}

func (pt *peerTracker) onRemove(host string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if !pt.currentPeers[host] {
		pt.t.Errorf("Invalid remove: peer %q does not exist", host)
	}
	delete(pt.currentPeers, host)
	select {
	case pt.onChange <- struct{}{}:
	default:
	}
}

func (pt *peerTracker) hasPeers(expected []string) bool {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if len(pt.currentPeers) != len(expected) {
		return false
	}
	for _, p := range expected {
		if !pt.currentPeers[p] {
			return false
		}
	}
	return true
}

// waitForPeers waits until the tracker has exactly the expected peers or timeout
func (pt *peerTracker) waitForPeers(expected []string, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		if pt.hasPeers(expected) {
			return true
		}
		select {
		case <-pt.onChange:
			// Check again
		case <-deadline:
			return false
		}
	}
}

func (pt *peerTracker) expectPeers(expected []string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	expectedSet := make(map[string]bool)
	for _, p := range expected {
		expectedSet[p] = true
	}

	// Check for missing peers
	for p := range expectedSet {
		if !pt.currentPeers[p] {
			pt.t.Errorf("Expected peer %q not found in current peers %v", p, pt.currentPeers)
		}
	}

	// Check for unexpected peers
	for p := range pt.currentPeers {
		if !expectedSet[p] {
			pt.t.Errorf("Unexpected peer %q found, expected %v", p, expected)
		}
	}
}

func TestController_AddRemovePeers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock := &mockProvider{peers: []string{"peer1:8080"}}
	tracker := newPeerTracker(t)

	controller := NewController(
		ctx,
		mock,
		1*time.Second,
		100*time.Millisecond,
		tracker.onAdd,
		tracker.onRemove,
	)

	go controller.Run()

	// Wait for initial peer to be added
	if !tracker.waitForPeers([]string{"peer1:8080"}, 2*time.Second) {
		t.Fatal("Timeout waiting for initial peer")
	}
	tracker.expectPeers([]string{"peer1:8080"})

	// Add another peer
	mock.setPeers([]string{"peer1:8080", "peer2:8080"})
	if !tracker.waitForPeers([]string{"peer1:8080", "peer2:8080"}, 2*time.Second) {
		t.Fatal("Timeout waiting for second peer")
	}
	tracker.expectPeers([]string{"peer1:8080", "peer2:8080"})

	// Remove first peer
	mock.setPeers([]string{"peer2:8080"})
	if !tracker.waitForPeers([]string{"peer2:8080"}, 2*time.Second) {
		t.Fatal("Timeout waiting for peer removal")
	}
	tracker.expectPeers([]string{"peer2:8080"})

	// Remove all peers
	mock.setPeers([]string{})
	if !tracker.waitForPeers([]string{}, 2*time.Second) {
		t.Fatal("Timeout waiting for all peers to be removed")
	}
	tracker.expectPeers([]string{})

	cancel()
}

func TestController_StopAll(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock := &mockProvider{peers: []string{"peer1:8080", "peer2:8080", "peer3:8080"}}
	tracker := newPeerTracker(t)

	controller := NewController(
		ctx,
		mock,
		1*time.Second,
		100*time.Millisecond,
		tracker.onAdd,
		tracker.onRemove,
	)

	go controller.Run()

	// Wait for all peers to be added
	if !tracker.waitForPeers([]string{"peer1:8080", "peer2:8080", "peer3:8080"}, 2*time.Second) {
		t.Fatal("Timeout waiting for peers to be added")
	}
	tracker.expectPeers([]string{"peer1:8080", "peer2:8080", "peer3:8080"})

	// Cancel context to trigger stopAll
	cancel()

	// Wait for all peers to be removed
	if !tracker.waitForPeers([]string{}, 2*time.Second) {
		t.Fatal("Timeout waiting for peers to be removed after cancel")
	}
	tracker.expectPeers([]string{})
}

func TestController_GetPeersWithTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock := &mockProvider{peers: []string{"peer1:8080"}}

	controller := NewController(
		ctx,
		mock,
		100*time.Millisecond,
		1*time.Second,
		func(host string) {},
		func(host string) {},
	)

	peers, err := controller.getPeers()
	if err != nil {
		t.Fatalf("getPeers() failed: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("Expected 1 peer, got %d", len(peers))
	}
}

func TestController_Reconcile(t *testing.T) {
	ctx := context.Background()

	mock := &mockProvider{}
	tracker := newPeerTracker(t)

	controller := NewController(
		ctx,
		mock,
		1*time.Second,
		1*time.Second,
		tracker.onAdd,
		tracker.onRemove,
	)

	// First reconcile: add 3 peers
	controller.reconcile([]string{"peer1", "peer2", "peer3"})
	tracker.expectPeers([]string{"peer1", "peer2", "peer3"})

	// Second reconcile: add 1, keep 2, remove 1
	controller.reconcile([]string{"peer2", "peer3", "peer4"})
	tracker.expectPeers([]string{"peer2", "peer3", "peer4"})

	// Third reconcile: remove all
	controller.reconcile([]string{})
	tracker.expectPeers([]string{})

	// Fourth reconcile: add new peers after empty
	controller.reconcile([]string{"peer5", "peer6"})
	tracker.expectPeers([]string{"peer5", "peer6"})
}

func TestController_RunWithInitialError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock := &mockProvider{peers: []string{"peer1:8080"}}
	mock.setErrorCount(1) // First call to GetPeers will fail

	tracker := newPeerTracker(t)
	controller := NewController(
		ctx,
		mock,
		100*time.Millisecond,
		100*time.Millisecond,
		tracker.onAdd,
		tracker.onRemove,
	)

	go controller.Run()

	// 초기 실패 후 재시도하여 피어가 추가되어야 함
	if !tracker.waitForPeers([]string{"peer1:8080"}, 300*time.Millisecond) {
		t.Error("Expected peer to be added after retry")
	}

	cancel()
}

func TestController_RunWithPeriodicError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock := &mockProvider{peers: []string{"peer1:8080"}}

	addedCount := atomic.Int32{}
	controller := NewController(
		ctx,
		mock,
		100*time.Millisecond,
		50*time.Millisecond,
		func(host string) { addedCount.Add(1) },
		func(host string) {},
	)

	go controller.Run()

	// Wait for initial peer to be added
	time.Sleep(100 * time.Millisecond)
	if addedCount.Load() != 1 {
		t.Error("Expected peer to be added initially")
	}

	// Set error for next periodic calls
	mock.setErrorCount(2)

	// Wait for periodic polls (should continue despite errors)
	time.Sleep(150 * time.Millisecond)

	cancel()
}

func TestController_GetPeersError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock := &mockProvider{}
	mock.setErrorCount(1)

	controller := NewController(
		ctx,
		mock,
		100*time.Millisecond,
		1*time.Second,
		func(host string) {},
		func(host string) {},
	)

	_, err := controller.getPeers()
	if err == nil {
		t.Error("Expected error from getPeers")
	}
}
