// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package transfer

import (
	"testing"
)

func TestChunkQueue_PushBack_Pop(t *testing.T) {
	queue := NewChunkQueue()

	// Test pop on empty queue
	_, ok := queue.Pop()
	if ok {
		t.Error("Expected Pop to return false on empty queue")
	}

	// Push and pop single item
	queue.PushBack(1)
	val, ok := queue.Pop()
	if !ok {
		t.Error("Expected Pop to return true")
	}
	if val != 1 {
		t.Errorf("Expected value 1, got %d", val)
	}

	// Push multiple items
	queue.PushBack(10)
	queue.PushBack(20)
	queue.PushBack(30)

	// Pop in order
	val, ok = queue.Pop()
	if !ok || val != 10 {
		t.Errorf("Expected 10, got %d", val)
	}
	val, ok = queue.Pop()
	if !ok || val != 20 {
		t.Errorf("Expected 20, got %d", val)
	}
	val, ok = queue.Pop()
	if !ok || val != 30 {
		t.Errorf("Expected 30, got %d", val)
	}

	// Queue should be empty
	_, ok = queue.Pop()
	if ok {
		t.Error("Expected Pop to return false on empty queue")
	}
}

func TestChunkQueue_PushShuffledList(t *testing.T) {
	queue := NewChunkQueue()

	indices := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	originalIndices := make([]int, len(indices))
	copy(originalIndices, indices)

	queue.PushShuffledList(indices)

	// Collect all values and check for duplicates
	popped := make([]int, 0, len(originalIndices))
	seen := make(map[int]bool)
	for {
		val, ok := queue.Pop()
		if !ok {
			break
		}
		// Check for duplicate
		if seen[val] {
			t.Errorf("Duplicate value %d found in queue", val)
		}
		seen[val] = true
		popped = append(popped, val)
	}

	// All values should be present
	if len(popped) != len(originalIndices) {
		t.Errorf("Expected %d items, got %d", len(originalIndices), len(popped))
	}

	// Check all original values are present (order may differ)
	for _, val := range originalIndices {
		if !seen[val] {
			t.Errorf("Value %d missing from shuffled queue", val)
		}
	}

	// Check no extra values
	for val := range seen {
		found := false
		for _, orig := range originalIndices {
			if val == orig {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Unexpected value %d found in queue", val)
		}
	}
}

func TestChunkQueue_PushShuffle(t *testing.T) {
	queue := NewChunkQueue()

	// Add items with shuffle
	for i := 1; i <= 10; i++ {
		queue.PushShuffle(i)
	}

	// Collect all values and check for duplicates
	popped := make([]int, 0, 10)
	seen := make(map[int]bool)
	for {
		val, ok := queue.Pop()
		if !ok {
			break
		}
		// Check for duplicate
		if seen[val] {
			t.Errorf("Duplicate value %d found in queue", val)
		}
		seen[val] = true
		popped = append(popped, val)
	}

	// All values should be present
	if len(popped) != 10 {
		t.Errorf("Expected 10 items, got %d", len(popped))
	}

	// Check all values from 1-10 are present
	for i := 1; i <= 10; i++ {
		if !seen[i] {
			t.Errorf("Value %d missing from queue", i)
		}
	}

	// Check no extra values (outside 1-10 range)
	for val := range seen {
		if val < 1 || val > 10 {
			t.Errorf("Unexpected value %d found in queue (expected 1-10)", val)
		}
	}
}

func TestChunkQueue_Concurrent(t *testing.T) {
	// This test ensures there are no race conditions
	// Run with: go test -race
	queue := NewChunkQueue()

	done := make(chan bool)

	// Producer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			queue.PushBack(i)
		}
		done <- true
	}()

	// Consumer goroutine
	go func() {
		count := 0
		for count < 100 {
			if _, ok := queue.Pop(); ok {
				count++
			}
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Queue should be empty
	if _, ok := queue.Pop(); ok {
		t.Error("Queue should be empty")
	}
}
