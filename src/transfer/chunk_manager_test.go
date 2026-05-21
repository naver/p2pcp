// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package transfer

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/naver/p2pcp/storage"
	"github.com/naver/p2pcp/testutil"
	"github.com/naver/p2pcp/types"
)

// createTestStorage wraps testutil.CreateTestStorageDefault for backward compatibility
func createTestStorage(t *testing.T) (storage.Storage, func()) {
	stor, _, cleanup := testutil.CreateTestStorageDefault(t)
	return stor, cleanup
}

func TestNewChunkManager(t *testing.T) {
	stor, cleanup := createTestStorage(t)
	defer cleanup()

	chunkSize := int64(1024)
	maxFileCountPerChunk := 10

	manager, err := NewChunkManager(stor, nil, 0, chunkSize, maxFileCountPerChunk)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	if manager == nil {
		t.Fatal("Expected manager to be non-nil")
	}

	if manager.GetFileCount() != 3 {
		t.Errorf("Expected 3 files, got %d", manager.GetFileCount())
	}

	if manager.GetChunkCount() == 0 {
		t.Error("Expected at least one chunk")
	}
}

func TestChunkManager_GetManifestJson(t *testing.T) {
	stor, cleanup := createTestStorage(t)
	defer cleanup()

	manager, err := NewChunkManager(stor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	jsonData := manager.GetManifestJson()
	if len(jsonData) == 0 {
		t.Error("Expected non-empty manifest JSON")
	}

	// Verify it's valid JSON by checking it contains expected fields
	jsonStr := string(jsonData)
	if !strings.Contains(jsonStr, "majorVersion") {
		t.Error("Manifest JSON should contain majorVersion field")
	}
	if !strings.Contains(jsonStr, "chunkSize") {
		t.Error("Manifest JSON should contain chunkSize field")
	}
}

func TestChunkManager_GetManifestChecksum(t *testing.T) {
	stor, cleanup := createTestStorage(t)
	defer cleanup()

	manager, err := NewChunkManager(stor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	checksum := manager.GetManifestChecksum()
	if checksum == 0 {
		t.Error("Expected non-zero manifest checksum")
	}
}

func TestChunkManager_ChunkStatus(t *testing.T) {
	stor, cleanup := createTestStorage(t)
	defer cleanup()

	manager, err := NewChunkManager(stor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	chunkIndex := 0

	// Initial status should be Pending
	status := manager.GetChunkStatus(chunkIndex)
	if status != types.ChunkStatusPending {
		t.Errorf("Expected pending status, got %v", status)
	}

	// Update status to Downloading
	success := manager.UpdateChunkStatus(chunkIndex, types.ChunkStatusPending, types.ChunkStatusDownloading)
	if !success {
		t.Error("Expected UpdateChunkStatus to succeed")
	}

	// Verify status changed
	status = manager.GetChunkStatus(chunkIndex)
	if status != types.ChunkStatusDownloading {
		t.Errorf("Expected downloading status, got %v", status)
	}

	// Set status to Done
	now := time.Now()
	manager.SetChunkStatusDone(chunkIndex, now)

	status = manager.GetChunkStatus(chunkIndex)
	if status != types.ChunkStatusDone {
		t.Errorf("Expected done status, got %v", status)
	}

	// Set back to Pending
	manager.SetChunkStatusPending(chunkIndex)
	status = manager.GetChunkStatus(chunkIndex)
	if status != types.ChunkStatusPending {
		t.Errorf("Expected pending status, got %v", status)
	}
}

func TestChunkManager_GetChunkSize(t *testing.T) {
	stor, cleanup := createTestStorage(t)
	defer cleanup()

	manager, err := NewChunkManager(stor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	for i := 0; i < manager.GetChunkCount(); i++ {
		size := manager.GetChunkSize(i)
		if size <= 0 {
			t.Errorf("Chunk %d has invalid size: %d", i, size)
		}
		if size > 1024 {
			t.Errorf("Chunk %d size %d exceeds chunk size limit 1024", i, size)
		}
	}
}

func TestChunkManager_ChunkChecksum(t *testing.T) {
	stor, cleanup := createTestStorage(t)
	defer cleanup()

	manager, err := NewChunkManager(stor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	chunkIndex := 0

	// Initial checksum should be 0
	checksum := manager.GetChunkChecksum(chunkIndex)
	if checksum != 0 {
		t.Errorf("Expected initial checksum to be 0, got %d", checksum)
	}

	// Compute checksum
	computed, err := manager.ComputeChunkChecksum(stor, chunkIndex)
	if err != nil {
		t.Fatalf("ComputeChunkChecksum failed: %v", err)
	}
	if computed == 0 {
		t.Error("Expected non-zero computed checksum")
	}

	// Verify checksum was cached
	cached := manager.GetChunkChecksum(chunkIndex)
	if cached != computed {
		t.Errorf("Expected cached checksum %d, got %d", computed, cached)
	}

	// Computing again should return same value
	computed2, err := manager.ComputeChunkChecksum(stor, chunkIndex)
	if err != nil {
		t.Fatalf("ComputeChunkChecksum failed on second call: %v", err)
	}
	if computed2 != computed {
		t.Errorf("Expected same checksum on second computation, got %d vs %d", computed2, computed)
	}

	// Test SetChunkChecksum
	newChecksum := uint64(12345)
	manager.SetChunkChecksum(chunkIndex, newChecksum)
	if manager.GetChunkChecksum(chunkIndex) != newChecksum {
		t.Errorf("Expected checksum %d after Set, got %d", newChecksum, manager.GetChunkChecksum(chunkIndex))
	}
}

// createTestStorageWithChunks creates test storage that guarantees at least minChunks chunks
// when used with chunkSize and maxFileCountPerChunk=1
func createTestStorageWithChunks(t *testing.T, chunkSize int64, minChunks int) (storage.Storage, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "p2pcp-chunk-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create enough files to guarantee minChunks chunks
	for i := 0; i < minChunks; i++ {
		content := bytes.Repeat([]byte{byte('a' + i%26)}, int(chunkSize))
		path := filepath.Join(tmpDir, fmt.Sprintf("file_%d.txt", i))
		if err := os.WriteFile(path, content, 0644); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	factory, err := storage.NewFactory(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create storage factory: %v", err)
	}

	stor, err := factory.Create()
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create storage: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return stor, cleanup
}

func TestChunkManager_GetStatusDoneChunks(t *testing.T) {
	stor, cleanup := createTestStorage(t)
	defer cleanup()

	manager, err := NewChunkManager(stor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	// Initially no chunks are done
	baseTime := time.Now()
	doneChunks := manager.GetStatusDoneChunks(baseTime)
	if len(doneChunks) != 0 {
		t.Errorf("Expected 0 done chunks initially, got %d", len(doneChunks))
	}

	// Mark first chunk as done with time after baseTime
	manager.SetChunkStatusDone(0, baseTime.Add(1*time.Second))

	doneChunks = manager.GetStatusDoneChunks(baseTime)
	if len(doneChunks) != 1 {
		t.Fatalf("Expected 1 done chunk, got %d", len(doneChunks))
	}
	if doneChunks[0] != 0 {
		t.Errorf("Expected chunk 0 to be done, got %d", doneChunks[0])
	}
}

func TestChunkManager_GetStatusDoneChunks_TimeFiltering(t *testing.T) {
	tests := []struct {
		name       string
		offsets    []time.Duration // chunk index -> time offset from baseTime
		wantChunks []int
	}{
		{
			name:       "excludes_chunks_before_time",
			offsets:    []time.Duration{1 * time.Second, -1 * time.Second},
			wantChunks: []int{0},
		},
		{
			name:       "mixed_before_and_after",
			offsets:    []time.Duration{2 * time.Second, -1 * time.Second, 3 * time.Second},
			wantChunks: []int{0, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const chunkSize = int64(500)
			minChunks := len(tt.offsets)

			stor, cleanup := createTestStorageWithChunks(t, chunkSize, minChunks)
			defer cleanup()

			manager, err := NewChunkManager(stor, nil, 0, chunkSize, 1)
			if err != nil {
				t.Fatalf("NewChunkManager failed: %v", err)
			}

			baseTime := time.Now()
			for i, offset := range tt.offsets {
				manager.SetChunkStatusDone(i, baseTime.Add(offset))
			}

			got := manager.GetStatusDoneChunks(baseTime)

			if len(got) != len(tt.wantChunks) {
				t.Errorf("got %d chunks, want %d", len(got), len(tt.wantChunks))
			}

			wantSet := make(map[int]bool)
			for _, idx := range tt.wantChunks {
				wantSet[idx] = true
			}
			for _, idx := range got {
				if !wantSet[idx] {
					t.Errorf("unexpected chunk %d in result", idx)
				}
			}
		})
	}
}

func TestChunkManager_GetStatusDoneChunks_BoundaryCondition(t *testing.T) {
	stor, cleanup := createTestStorage(t)
	defer cleanup()

	manager, err := NewChunkManager(stor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	baseTime := time.Now()

	// Set chunk updated at exactly the same time as baseTime
	// After() returns false when times are equal, so this should NOT be included
	manager.SetChunkStatusDone(0, baseTime)

	doneChunks := manager.GetStatusDoneChunks(baseTime)
	if len(doneChunks) != 0 {
		t.Errorf("Chunk updated at exactly baseTime should NOT be included (After() is exclusive), got %d chunks", len(doneChunks))
	}

	// Set chunk updated 1 nanosecond after baseTime - should be included
	manager.SetChunkStatusDone(0, baseTime.Add(1*time.Nanosecond))

	doneChunks = manager.GetStatusDoneChunks(baseTime)
	if len(doneChunks) != 1 {
		t.Errorf("Chunk updated 1ns after baseTime should be included, got %d chunks", len(doneChunks))
	}
}

func TestChunkManager_GetStatusDoneChunks_OnlyDoneStatus(t *testing.T) {
	const chunkSize = int64(500)
	const minChunks = 2

	stor, cleanup := createTestStorageWithChunks(t, chunkSize, minChunks)
	defer cleanup()

	manager, err := NewChunkManager(stor, nil, 0, chunkSize, 1)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	if manager.GetChunkCount() < minChunks {
		t.Fatalf("Test setup error: expected at least %d chunks, got %d", minChunks, manager.GetChunkCount())
	}

	baseTime := time.Now()

	// Chunk 0: Done status with time after baseTime
	manager.SetChunkStatusDone(0, baseTime.Add(1*time.Second))

	// Chunk 1: Change to Downloading status (not Done)
	manager.UpdateChunkStatus(1, types.ChunkStatusPending, types.ChunkStatusDownloading)

	doneChunks := manager.GetStatusDoneChunks(baseTime)

	// Only chunk 0 should be returned (Done status)
	if len(doneChunks) != 1 {
		t.Fatalf("Expected 1 done chunk, got %d", len(doneChunks))
	}

	if doneChunks[0] != 0 {
		t.Errorf("Expected chunk 0 in result, got %d", doneChunks[0])
	}
}

func TestChunkManager_WriteChunk(t *testing.T) {
	// Create source storage
	srcStor, srcCleanup := createTestStorage(t)
	defer srcCleanup()

	// Create destination storage
	dstStor, _, dstCleanup := testutil.CreateEmptyTestStorage(t)
	defer dstCleanup()

	// Create chunk manager from source
	manager, err := NewChunkManager(srcStor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	// Prepare destination files
	manager.manifest.Dirs = []types.DirEntry{}
	manager.manifest.Symlinks = []types.SymlinkEntry{}
	manager.manifest.EmptyFiles = []types.EmptyFileEntry{}

	for i := range manager.manifest.Files {
		err := dstStor.PrepareEntry(&manager.manifest.Files[i])
		if err != nil {
			t.Fatalf("Failed to prepare file: %v", err)
		}
	}

	// Test writing first chunk
	chunkIndex := 0
	reader := manager.GetChunkReader(srcStor, chunkIndex)

	completedFiles := []string{}
	onFileComplete := func(path string, size int64) {
		completedFiles = append(completedFiles, path)
	}

	written, err := manager.WriteChunk(dstStor, chunkIndex, reader, 0, onFileComplete)
	reader.Close()

	if err != nil {
		t.Fatalf("WriteChunk failed: %v", err)
	}

	expectedSize := manager.GetChunkSize(chunkIndex)
	if written != expectedSize {
		t.Errorf("Expected to write %d bytes, wrote %d", expectedSize, written)
	}
}

func TestChunkManager_WriteChunk_ChecksumVerification(t *testing.T) {
	srcStor, srcCleanup := createTestStorage(t)
	defer srcCleanup()

	dstStor, _, dstCleanup := testutil.CreateEmptyTestStorage(t)
	defer dstCleanup()

	manager, err := NewChunkManager(srcStor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	manager.manifest.Dirs = []types.DirEntry{}
	manager.manifest.Symlinks = []types.SymlinkEntry{}
	manager.manifest.EmptyFiles = []types.EmptyFileEntry{}

	for i := range manager.manifest.Files {
		err := dstStor.PrepareEntry(&manager.manifest.Files[i])
		if err != nil {
			t.Fatalf("Failed to prepare file: %v", err)
		}
	}

	chunkIndex := 0
	onFileComplete := func(path string, size int64) {}

	t.Run("valid checksum", func(t *testing.T) {
		reader := manager.GetChunkReader(srcStor, chunkIndex)
		defer reader.Close()

		expectedChecksum, err := manager.ComputeChunkChecksum(srcStor, chunkIndex)
		if err != nil {
			t.Fatalf("ComputeChunkChecksum failed: %v", err)
		}

		_, err = manager.WriteChunk(dstStor, chunkIndex, reader, expectedChecksum, onFileComplete)
		if err != nil {
			t.Errorf("WriteChunk with valid checksum should succeed, got: %v", err)
		}
	})

	t.Run("invalid checksum", func(t *testing.T) {
		reader := manager.GetChunkReader(srcStor, chunkIndex)
		defer reader.Close()

		expectedChecksum, err := manager.ComputeChunkChecksum(srcStor, chunkIndex)
		if err != nil {
			t.Fatalf("ComputeChunkChecksum failed: %v", err)
		}
		invalidChecksum := expectedChecksum + 1

		_, err = manager.WriteChunk(dstStor, chunkIndex, reader, invalidChecksum, onFileComplete)
		if err == nil || err.Error() != "checksum mismatch" {
			t.Errorf("WriteChunk with invalid checksum should return checksum mismatch error, got: %v", err)
		}
	})

	t.Run("zero checksum skips verification", func(t *testing.T) {
		// Use fake data that differs from the actual chunk content.
		chunkSize := manager.GetChunkSize(chunkIndex)
		fakeData := make([]byte, chunkSize)
		for i := range fakeData {
			fakeData[i] = 0xFF
		}

		// With actual checksum, fake data should fail verification.
		expectedChecksum, err := manager.ComputeChunkChecksum(srcStor, chunkIndex)
		if err != nil {
			t.Fatalf("ComputeChunkChecksum failed: %v", err)
		}
		fakeReader := bytes.NewReader(fakeData)
		_, err = manager.WriteChunk(dstStor, chunkIndex, fakeReader, expectedChecksum, onFileComplete)
		if err == nil || err.Error() != "checksum mismatch" {
			t.Errorf("WriteChunk with fake data and real checksum should fail, got: %v", err)
		}

		// With checksum=0, verification is skipped, so fake data should succeed.
		fakeReader = bytes.NewReader(fakeData)
		_, err = manager.WriteChunk(dstStor, chunkIndex, fakeReader, 0, onFileComplete)
		if err != nil {
			t.Errorf("WriteChunk with zero checksum should skip verification, got: %v", err)
		}
	})
}

func TestChunkManager_GetChunkReader(t *testing.T) {
	stor, cleanup := createTestStorage(t)
	defer cleanup()

	manager, err := NewChunkManager(stor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	// Test reading first chunk
	chunkIndex := 0
	reader := manager.GetChunkReader(stor, chunkIndex)
	defer reader.Close()

	if reader == nil {
		t.Fatal("Expected non-nil reader")
	}

	// Read all data
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Failed to read chunk: %v", err)
	}

	expectedSize := manager.GetChunkSize(chunkIndex)
	if int64(len(data)) != expectedSize {
		t.Errorf("Expected to read %d bytes, read %d", expectedSize, len(data))
	}
}
