// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package types

import (
	"testing"
	"time"
)

func TestBuildChunks(t *testing.T) {
	tests := []struct {
		name                    string
		files                   []FileEntry
		chunkSize               int64
		maxFileCount            int
		expectedChunks          [][]FileRange
		expectedChunkSizes      []int64
		expectedSortedNames     []string // nil to skip check
		expectedRemainingCounts []int32
	}{
		{
			name:                    "empty files list",
			files:                   []FileEntry{},
			chunkSize:               1024,
			maxFileCount:            10,
			expectedChunks:          [][]FileRange{},
			expectedChunkSizes:      []int64{},
			expectedSortedNames:     nil,
			expectedRemainingCounts: []int32{},
		},
		{
			name: "single small file not split",
			files: []FileEntry{
				{Name: "file1.txt", Size: 100, Perm: 0644},
			},
			chunkSize:    1024,
			maxFileCount: 10,
			expectedChunks: [][]FileRange{
				{{FileIndex: 0, StartOffset: 0, EndOffset: 100}},
			},
			expectedChunkSizes:      []int64{100},
			expectedSortedNames:     nil,
			expectedRemainingCounts: []int32{1},
		},
		{
			name: "multiple small files fitting in one chunk",
			files: []FileEntry{
				{Name: "a.txt", Size: 100, Perm: 0644},
				{Name: "b.txt", Size: 200, Perm: 0644},
				{Name: "c.txt", Size: 300, Perm: 0644},
			},
			chunkSize:    1024,
			maxFileCount: 10,
			expectedChunks: [][]FileRange{
				{
					{FileIndex: 0, StartOffset: 0, EndOffset: 100},
					{FileIndex: 1, StartOffset: 0, EndOffset: 200},
					{FileIndex: 2, StartOffset: 0, EndOffset: 300},
				},
			},
			expectedChunkSizes:      []int64{600},
			expectedSortedNames:     nil,
			expectedRemainingCounts: []int32{1, 1, 1},
		},
		{
			name: "single large file split across multiple chunks",
			files: []FileEntry{
				{Name: "large.bin", Size: 3000, Perm: 0644},
			},
			chunkSize:    1000,
			maxFileCount: 10,
			expectedChunks: [][]FileRange{
				{{FileIndex: 0, StartOffset: 0, EndOffset: 1000}},
				{{FileIndex: 0, StartOffset: 1000, EndOffset: 2000}},
				{{FileIndex: 0, StartOffset: 2000, EndOffset: 3000}},
			},
			expectedChunkSizes:      []int64{1000, 1000, 1000},
			expectedSortedNames:     nil,
			expectedRemainingCounts: []int32{3},
		},
		{
			name: "multiple files with some split across chunks",
			files: []FileEntry{
				{Name: "large.bin", Size: 2500, Perm: 0644},
				{Name: "small.txt", Size: 500, Perm: 0644},
			},
			chunkSize:    1000,
			maxFileCount: 10,
			expectedChunks: [][]FileRange{
				{{FileIndex: 0, StartOffset: 0, EndOffset: 1000}},
				{{FileIndex: 0, StartOffset: 1000, EndOffset: 2000}},
				{
					{FileIndex: 0, StartOffset: 2000, EndOffset: 2500},
					{FileIndex: 1, StartOffset: 0, EndOffset: 500},
				},
			},
			expectedChunkSizes:      []int64{1000, 1000, 1000},
			expectedSortedNames:     nil,
			expectedRemainingCounts: []int32{3, 1},
		},
		{
			name: "max file count per chunk limit",
			files: []FileEntry{
				{Name: "a.txt", Size: 10, Perm: 0644},
				{Name: "b.txt", Size: 10, Perm: 0644},
				{Name: "c.txt", Size: 10, Perm: 0644},
				{Name: "d.txt", Size: 10, Perm: 0644},
			},
			chunkSize:    1024,
			maxFileCount: 2,
			expectedChunks: [][]FileRange{
				{
					{FileIndex: 0, StartOffset: 0, EndOffset: 10},
					{FileIndex: 1, StartOffset: 0, EndOffset: 10},
				},
				{
					{FileIndex: 2, StartOffset: 0, EndOffset: 10},
					{FileIndex: 3, StartOffset: 0, EndOffset: 10},
				},
			},
			expectedChunkSizes:      []int64{20, 20},
			expectedSortedNames:     nil,
			expectedRemainingCounts: []int32{1, 1, 1, 1},
		},
		{
			name: "files are sorted by name before chunking",
			files: []FileEntry{
				{Name: "z.txt", Size: 100, Perm: 0644},
				{Name: "a.txt", Size: 200, Perm: 0644},
				{Name: "m.txt", Size: 300, Perm: 0644},
			},
			chunkSize:    1024,
			maxFileCount: 10,
			expectedChunks: [][]FileRange{
				{
					{FileIndex: 0, StartOffset: 0, EndOffset: 200}, // a.txt
					{FileIndex: 1, StartOffset: 0, EndOffset: 300}, // m.txt
					{FileIndex: 2, StartOffset: 0, EndOffset: 100}, // z.txt
				},
			},
			expectedChunkSizes:      []int64{600},
			expectedSortedNames:     []string{"a.txt", "m.txt", "z.txt"},
			expectedRemainingCounts: []int32{1, 1, 1},
		},
		{
			name: "file size exactly equals chunk size",
			files: []FileEntry{
				{Name: "exact.bin", Size: 1000, Perm: 0644},
			},
			chunkSize:    1000,
			maxFileCount: 10,
			expectedChunks: [][]FileRange{
				{{FileIndex: 0, StartOffset: 0, EndOffset: 1000}},
			},
			expectedChunkSizes:      []int64{1000},
			expectedSortedNames:     nil,
			expectedRemainingCounts: []int32{1},
		},
		{
			name: "file size slightly larger than chunk size",
			files: []FileEntry{
				{Name: "slight.bin", Size: 1001, Perm: 0644},
			},
			chunkSize:    1000,
			maxFileCount: 10,
			expectedChunks: [][]FileRange{
				{{FileIndex: 0, StartOffset: 0, EndOffset: 1000}},
				{{FileIndex: 0, StartOffset: 1000, EndOffset: 1001}},
			},
			expectedChunkSizes:      []int64{1000, 1},
			expectedSortedNames:     nil,
			expectedRemainingCounts: []int32{2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Copy files to avoid mutation between tests
			files := make([]FileEntry, len(tt.files))
			copy(files, tt.files)

			chunks := BuildChunks(files, tt.chunkSize, tt.maxFileCount)

			// Check chunk count
			if len(chunks) != len(tt.expectedChunks) {
				t.Fatalf("chunk count: got %d, want %d", len(chunks), len(tt.expectedChunks))
			}

			// Check each chunk's FileRanges and Size
			for i := range chunks {
				if len(chunks[i].FileRanges) != len(tt.expectedChunks[i]) {
					t.Fatalf("chunk[%d] FileRanges count: got %d, want %d",
						i, len(chunks[i].FileRanges), len(tt.expectedChunks[i]))
				}
				for j, fr := range chunks[i].FileRanges {
					expected := tt.expectedChunks[i][j]
					if fr.FileIndex != expected.FileIndex ||
						fr.StartOffset != expected.StartOffset ||
						fr.EndOffset != expected.EndOffset {
						t.Errorf("chunk[%d].FileRanges[%d]: got {%d, %d, %d}, want {%d, %d, %d}",
							i, j,
							fr.FileIndex, fr.StartOffset, fr.EndOffset,
							expected.FileIndex, expected.StartOffset, expected.EndOffset)
					}
				}

				// Check chunk Size()
				if chunks[i].Size() != tt.expectedChunkSizes[i] {
					t.Errorf("chunk[%d].Size(): got %d, want %d",
						i, chunks[i].Size(), tt.expectedChunkSizes[i])
				}
			}

			// Check file sorting (if expectedSortedNames is provided)
			if tt.expectedSortedNames != nil {
				for i, expectedName := range tt.expectedSortedNames {
					if files[i].Name != expectedName {
						t.Errorf("files[%d].Name after sort: got %q, want %q",
							i, files[i].Name, expectedName)
					}
				}
			}

			// Check remainingChunkCount for each file
			for i, expected := range tt.expectedRemainingCounts {
				actual := files[i].remainingChunkCount.Load()
				if actual != expected {
					t.Errorf("files[%d].remainingChunkCount: got %d, want %d", i, actual, expected)
				}
			}
		})
	}
}

func TestChunk_Size(t *testing.T) {
	t.Run("empty chunk", func(t *testing.T) {
		chunk := Chunk{}
		if chunk.Size() != 0 {
			t.Errorf("Expected size 0, got %d", chunk.Size())
		}
	})

	t.Run("single file range", func(t *testing.T) {
		chunk := Chunk{
			FileRanges: []FileRange{
				{FileIndex: 0, StartOffset: 0, EndOffset: 100},
			},
		}
		if chunk.Size() != 100 {
			t.Errorf("Expected size 100, got %d", chunk.Size())
		}
	})

	t.Run("multiple file ranges", func(t *testing.T) {
		chunk := Chunk{
			FileRanges: []FileRange{
				{FileIndex: 0, StartOffset: 0, EndOffset: 100},
				{FileIndex: 1, StartOffset: 50, EndOffset: 150},
				{FileIndex: 2, StartOffset: 0, EndOffset: 200},
			},
		}
		expected := int64(100 + 100 + 200)
		if chunk.Size() != expected {
			t.Errorf("Expected size %d, got %d", expected, chunk.Size())
		}
	})
}

func TestChunk_Status(t *testing.T) {
	t.Run("default status is pending", func(t *testing.T) {
		chunk := Chunk{}
		if chunk.GetStatus() != ChunkStatusPending {
			t.Errorf("Expected pending status, got %v", chunk.GetStatus())
		}
	})

	t.Run("set and get status", func(t *testing.T) {
		chunk := Chunk{}
		chunk.SetStatus(ChunkStatusDownloading)
		if chunk.GetStatus() != ChunkStatusDownloading {
			t.Errorf("Expected downloading status, got %v", chunk.GetStatus())
		}
		chunk.SetStatus(ChunkStatusDone)
		if chunk.GetStatus() != ChunkStatusDone {
			t.Errorf("Expected done status, got %v", chunk.GetStatus())
		}
	})

	t.Run("update status with CAS", func(t *testing.T) {
		chunk := Chunk{}
		// Should succeed: Pending -> Downloading
		if !chunk.UpdateStatus(ChunkStatusPending, ChunkStatusDownloading) {
			t.Error("Expected UpdateStatus to succeed")
		}
		if chunk.GetStatus() != ChunkStatusDownloading {
			t.Errorf("Expected downloading status, got %v", chunk.GetStatus())
		}
		// Should fail: Pending -> Done (current status is Downloading)
		if chunk.UpdateStatus(ChunkStatusPending, ChunkStatusDone) {
			t.Error("Expected UpdateStatus to fail with wrong old status")
		}
		if chunk.GetStatus() != ChunkStatusDownloading {
			t.Errorf("Status should remain downloading, got %v", chunk.GetStatus())
		}
		// Should succeed: Downloading -> Done
		if !chunk.UpdateStatus(ChunkStatusDownloading, ChunkStatusDone) {
			t.Error("Expected UpdateStatus to succeed")
		}
		if chunk.GetStatus() != ChunkStatusDone {
			t.Errorf("Expected done status, got %v", chunk.GetStatus())
		}
	})
}

func TestChunk_UpdatedAt(t *testing.T) {
	chunk := Chunk{}
	now := time.Now()
	chunk.SetUpdatedAt(now)
	retrieved := chunk.GetUpdatedAt()
	// Compare with some tolerance since time precision might differ
	if retrieved.Sub(now).Abs() > time.Millisecond {
		t.Errorf("Expected time %v, got %v", now, retrieved)
	}
}

func TestChunk_Copy(t *testing.T) {
	original := Chunk{
		FileRanges: []FileRange{
			{FileIndex: 0, StartOffset: 0, EndOffset: 100},
			{FileIndex: 1, StartOffset: 0, EndOffset: 200},
		},
	}
	original.SetStatus(ChunkStatusDownloading)
	original.SetUpdatedAt(time.Now())

	copied := original.copy()

	// FileRanges should be copied
	if len(copied.FileRanges) != len(original.FileRanges) {
		t.Errorf("Expected %d file ranges, got %d", len(original.FileRanges), len(copied.FileRanges))
	}
	for i := range original.FileRanges {
		if copied.FileRanges[i] != original.FileRanges[i] {
			t.Errorf("FileRange %d mismatch", i)
		}
	}

	// Status should be reset to default (Pending)
	if copied.GetStatus() != ChunkStatusPending {
		t.Errorf("Expected copied chunk to have pending status, got %v", copied.GetStatus())
	}
}
