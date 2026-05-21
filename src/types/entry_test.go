// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package types

import (
	"testing"
)

func TestEntry_GetName(t *testing.T) {
	tests := []struct {
		name     string
		entry    Entry
		expected string
	}{
		{"DirEntry", &DirEntry{Name: "testdir", Perm: 0755}, "testdir"},
		{"SymlinkEntry", &SymlinkEntry{Name: "testlink", SymlinkTo: "/target"}, "testlink"},
		{"EmptyFileEntry", &EmptyFileEntry{Name: "empty.txt", Perm: 0644}, "empty.txt"},
		{"FileEntry", &FileEntry{Name: "file.txt", Size: 1024, Perm: 0644}, "file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.entry.GetName(); got != tt.expected {
				t.Errorf("Expected name '%s', got '%s'", tt.expected, got)
			}
		})
	}
}

func TestFileEntry_Copy(t *testing.T) {
	original := FileEntry{
		Name: "test.txt",
		Size: 2048,
		Perm: 0644,
	}
	original.IncrementRemainingChunkCount()
	original.IncrementRemainingChunkCount()

	copied := original.Copy()

	if copied.Name != original.Name {
		t.Errorf("Expected name '%s', got '%s'", original.Name, copied.Name)
	}
	if copied.Size != original.Size {
		t.Errorf("Expected size %d, got %d", original.Size, copied.Size)
	}
	if copied.Perm != original.Perm {
		t.Errorf("Expected perm %v, got %v", original.Perm, copied.Perm)
	}
	// Copied entry should have zero remaining chunk count
	if copied.remainingChunkCount.Load() != 0 {
		t.Errorf("Expected copied entry to have 0 remaining chunks, got %d", copied.remainingChunkCount.Load())
	}
}

func TestFileEntry_RemainingChunkCount(t *testing.T) {
	entry := FileEntry{Name: "test.txt", Size: 1024, Perm: 0644}

	// Initial count should be 0
	if entry.remainingChunkCount.Load() != 0 {
		t.Errorf("Expected initial count 0, got %d", entry.remainingChunkCount.Load())
	}

	// Increment
	count := entry.IncrementRemainingChunkCount()
	if count != 1 {
		t.Errorf("Expected count 1 after increment, got %d", count)
	}
	count = entry.IncrementRemainingChunkCount()
	if count != 2 {
		t.Errorf("Expected count 2 after second increment, got %d", count)
	}

	// Decrement
	count = entry.DecrementRemainingChunkCount()
	if count != 1 {
		t.Errorf("Expected count 1 after decrement, got %d", count)
	}
	count = entry.DecrementRemainingChunkCount()
	if count != 0 {
		t.Errorf("Expected count 0 after second decrement, got %d", count)
	}
}
