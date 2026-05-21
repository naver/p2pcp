// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package utils

import (
	"testing"
	"time"
)

func TestAtomicDuration(t *testing.T) {
	var ad AtomicDuration

	// Test Load on uninitialized value
	if ad.Load() != 0 {
		t.Errorf("Expected initial value 0, got %v", ad.Load())
	}

	// Test Store and Load
	expected := 5 * time.Second
	ad.Store(expected)
	if ad.Load() != expected {
		t.Errorf("Expected %v, got %v", expected, ad.Load())
	}

	// Test Swap
	old := ad.Swap(10 * time.Second)
	if old != expected {
		t.Errorf("Expected Swap to return old value %v, got %v", expected, old)
	}
	if ad.Load() != 10*time.Second {
		t.Errorf("Expected current value 10s, got %v", ad.Load())
	}

	// Test CompareAndSwap - success case
	if !ad.CompareAndSwap(10*time.Second, 15*time.Second) {
		t.Error("Expected CompareAndSwap to succeed")
	}
	if ad.Load() != 15*time.Second {
		t.Errorf("Expected value 15s after CAS, got %v", ad.Load())
	}

	// Test CompareAndSwap - failure case
	if ad.CompareAndSwap(10*time.Second, 20*time.Second) {
		t.Error("Expected CompareAndSwap to fail with wrong old value")
	}
	if ad.Load() != 15*time.Second {
		t.Errorf("Expected value to remain 15s after failed CAS, got %v", ad.Load())
	}

	// Test Add
	result := ad.Add(5 * time.Second)
	if result != 20*time.Second {
		t.Errorf("Expected Add to return 20s, got %v", result)
	}
	if ad.Load() != 20*time.Second {
		t.Errorf("Expected value 20s after Add, got %v", ad.Load())
	}

	// Test Add with negative value
	result = ad.Add(-10 * time.Second)
	if result != 10*time.Second {
		t.Errorf("Expected Add to return 10s, got %v", result)
	}
}

func TestAtomicTime(t *testing.T) {
	var at AtomicTime

	// Test Swap on uninitialized value
	expected := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	old := at.Swap(expected)
	if !old.IsZero() {
		t.Errorf("Expected Swap to return zero time for uninitialized value, got %v", old)
	}
	if !at.Load().Equal(expected) {
		t.Errorf("Expected value %v after Swap, got %v", expected, at.Load())
	}

	// Test Store and Load
	expected = time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	at.Store(expected)
	if !at.Load().Equal(expected) {
		t.Errorf("Expected %v, got %v", expected, at.Load())
	}

	// Test Swap
	newTime := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	old = at.Swap(newTime)
	if !old.Equal(expected) {
		t.Errorf("Expected Swap to return old value %v, got %v", expected, old)
	}
	if !at.Load().Equal(newTime) {
		t.Errorf("Expected current value %v, got %v", newTime, at.Load())
	}

	// Test CompareAndSwap - success case
	newerTime := time.Date(2024, 12, 1, 12, 0, 0, 0, time.UTC)
	if !at.CompareAndSwap(newTime, newerTime) {
		t.Error("Expected CompareAndSwap to succeed")
	}
	if !at.Load().Equal(newerTime) {
		t.Errorf("Expected value %v after CAS, got %v", newerTime, at.Load())
	}

	// Test CompareAndSwap - failure case
	wrongTime := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	if at.CompareAndSwap(wrongTime, expected) {
		t.Error("Expected CompareAndSwap to fail with wrong old value")
	}
	if !at.Load().Equal(newerTime) {
		t.Errorf("Expected value to remain %v after failed CAS, got %v", newerTime, at.Load())
	}
}

func TestAtomicDuration_Concurrent(t *testing.T) {
	// This test is mainly for race detection
	// Run with: go test -race
	var ad AtomicDuration
	ad.Store(0)

	done := make(chan bool)

	// Multiple goroutines incrementing
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				ad.Add(1 * time.Millisecond)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	expected := 1000 * time.Millisecond
	if ad.Load() != expected {
		t.Errorf("Expected %v after concurrent adds, got %v", expected, ad.Load())
	}
}

func TestAtomicTime_Concurrent(t *testing.T) {
	// This test is mainly for race detection
	// Run with: go test -race
	var at AtomicTime

	done := make(chan bool)

	// Multiple goroutines storing
	for i := 0; i < 10; i++ {
		go func(idx int) {
			for j := 0; j < 100; j++ {
				at.Store(time.Date(2024, 1, 1, idx, j, 0, 0, time.UTC))
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Just check that we have some value
	loaded := at.Load()
	if loaded.IsZero() {
		t.Error("Expected non-zero time after concurrent stores")
	}
}
