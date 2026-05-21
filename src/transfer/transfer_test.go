// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package transfer

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/naver/p2pcp/testutil"
	"github.com/naver/p2pcp/types"
)

func TestSetupTransfer(t *testing.T) {
	srcStor, _, srcCleanup := testutil.CreateTestStorageWithEntries(t)
	defer srcCleanup()

	// Create chunk manager from source
	manager, err := NewChunkManager(srcStor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	// Create destination storage
	dstStor, dstDir, dstCleanup := testutil.CreateEmptyTestStorage(t)
	defer dstCleanup()

	// Run SetupTransfer
	err = SetupTransfer(manager, dstStor)
	if err != nil {
		t.Fatalf("SetupTransfer failed: %v", err)
	}

	// Verify directories were created with correct permissions
	for _, dir := range manager.manifest.Dirs {
		dirPath := filepath.Join(dstDir, dir.Name)
		info, err := os.Stat(dirPath)
		if err != nil {
			t.Errorf("Directory '%s' was not created: %v", dir.Name, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("Expected '%s' to be a directory, but it's not", dir.Name)
		}
	}

	// Verify symlinks were created with correct targets
	for _, symlink := range manager.manifest.Symlinks {
		symlinkPath := filepath.Join(dstDir, symlink.Name)
		info, err := os.Lstat(symlinkPath)
		if err != nil {
			t.Errorf("Symlink '%s' was not created: %v", symlink.Name, err)
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("Expected '%s' to be a symlink, but it's not", symlink.Name)
			continue
		}
		target, err := os.Readlink(symlinkPath)
		if err != nil {
			t.Errorf("Failed to read symlink '%s': %v", symlink.Name, err)
			continue
		}
		if target != symlink.SymlinkTo {
			t.Errorf("Symlink '%s' target mismatch: expected '%s', got '%s'", symlink.Name, symlink.SymlinkTo, target)
		}
	}

	// Verify empty files were created
	for _, emptyFile := range manager.manifest.EmptyFiles {
		emptyPath := filepath.Join(dstDir, emptyFile.Name)
		info, err := os.Stat(emptyPath)
		if err != nil {
			t.Errorf("Empty file '%s' was not created: %v", emptyFile.Name, err)
			continue
		}
		if info.Size() != 0 {
			t.Errorf("Empty file '%s' should have size 0, got %d", emptyFile.Name, info.Size())
		}
	}

	// Verify files were prepared
	for i := range manager.manifest.Files {
		file := &manager.manifest.Files[i]
		filePath := filepath.Join(dstDir, file.Name)
		_, err := os.Stat(filePath)
		if err != nil {
			t.Errorf("File '%s' was not prepared: %v", file.Name, err)
		}
	}
}

func TestCompleteTransfer(t *testing.T) {
	// Get expected file contents from testutil to stay in sync
	expectedFiles := testutil.WithEntriesTestStorageOptions().Files

	srcStor, _, srcCleanup := testutil.CreateTestStorageWithEntries(t)
	defer srcCleanup()

	// Create chunk manager from source
	manager, err := NewChunkManager(srcStor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	// Create destination storage
	dstStor, dstDir, dstCleanup := testutil.CreateEmptyTestStorage(t)
	defer dstCleanup()

	// Setup first
	err = SetupTransfer(manager, dstStor)
	if err != nil {
		t.Fatalf("SetupTransfer failed: %v", err)
	}

	// Write all chunks
	for i := 0; i < manager.GetChunkCount(); i++ {
		reader := manager.GetChunkReader(srcStor, i)
		_, err := manager.WriteChunk(dstStor, i, reader, 0, func(path string, size int64) {})
		reader.Close()
		if err != nil {
			t.Fatalf("WriteChunk %d failed: %v", i, err)
		}
	}

	// Run CompleteTransfer
	err = CompleteTransfer(manager, dstStor)
	if err != nil {
		t.Fatalf("CompleteTransfer failed: %v", err)
	}

	// Verify files have correct permissions and content
	for i := range manager.manifest.Files {
		file := &manager.manifest.Files[i]
		filePath := filepath.Join(dstDir, file.Name)
		info, err := os.Stat(filePath)
		if err != nil {
			t.Errorf("File '%s' not found after complete: %v", file.Name, err)
			continue
		}

		// Permission check
		if info.Mode().Perm() != file.Perm {
			t.Errorf("File '%s' permission mismatch: expected %o, got %o", file.Name, file.Perm, info.Mode().Perm())
		}

		// Size check
		if info.Size() != file.Size {
			t.Errorf("File '%s' size mismatch: expected %d, got %d", file.Name, file.Size, info.Size())
		}

		// Content check
		actualContent, err := os.ReadFile(filePath)
		if err != nil {
			t.Errorf("Failed to read file '%s': %v", file.Name, err)
			continue
		}
		expectedContent, exists := expectedFiles[file.Name]
		if exists && !bytes.Equal(actualContent, expectedContent) {
			t.Errorf("File '%s' content mismatch: expected %d bytes starting with %q, got %d bytes starting with %q",
				file.Name, len(expectedContent), expectedContent[:min(10, len(expectedContent))],
				len(actualContent), actualContent[:min(10, len(actualContent))])
		}
	}

	// Verify directories have correct permissions
	for _, dir := range manager.manifest.Dirs {
		dirPath := filepath.Join(dstDir, dir.Name)
		info, err := os.Stat(dirPath)
		if err != nil {
			t.Errorf("Directory '%s' not found after complete: %v", dir.Name, err)
			continue
		}
		if info.Mode().Perm() != dir.Perm {
			t.Errorf("Directory '%s' permission mismatch: expected %o, got %o", dir.Name, dir.Perm, info.Mode().Perm())
		}
	}

	// Verify symlinks
	for _, symlink := range manager.manifest.Symlinks {
		symlinkPath := filepath.Join(dstDir, symlink.Name)
		target, err := os.Readlink(symlinkPath)
		if err != nil {
			t.Errorf("Symlink '%s' not readable: %v", symlink.Name, err)
			continue
		}
		if target != symlink.SymlinkTo {
			t.Errorf("Symlink '%s' target mismatch: expected '%s', got '%s'", symlink.Name, symlink.SymlinkTo, target)
		}
	}

	// Verify empty files
	for _, emptyFile := range manager.manifest.EmptyFiles {
		emptyPath := filepath.Join(dstDir, emptyFile.Name)
		info, err := os.Stat(emptyPath)
		if err != nil {
			t.Errorf("Empty file '%s' not found: %v", emptyFile.Name, err)
			continue
		}
		if info.Size() != 0 {
			t.Errorf("Empty file '%s' should have size 0, got %d", emptyFile.Name, info.Size())
		}
		if info.Mode().Perm() != emptyFile.Perm {
			t.Errorf("Empty file '%s' permission mismatch: expected %o, got %o", emptyFile.Name, emptyFile.Perm, info.Mode().Perm())
		}
	}
}

func TestVerifyTransfer_Success(t *testing.T) {
	srcStor, _, srcCleanup := testutil.CreateTestStorageWithEntries(t)
	defer srcCleanup()

	// Create chunk manager from source
	manager, err := NewChunkManager(srcStor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	// Compute checksums for source chunks
	for i := 0; i < manager.GetChunkCount(); i++ {
		_, err := manager.ComputeChunkChecksum(srcStor, i)
		if err != nil {
			t.Fatalf("ComputeChunkChecksum failed: %v", err)
		}
	}

	// Verify against source (should succeed)
	isValid := VerifyTransfer(manager, srcStor)
	if !isValid {
		t.Error("VerifyTransfer should succeed for source storage")
	}
}

func TestVerifyTransfer_FullTransfer(t *testing.T) {
	// Get expected file contents from testutil to stay in sync
	expectedFiles := testutil.WithEntriesTestStorageOptions().Files

	srcStor, srcDir, srcCleanup := testutil.CreateTestStorageWithEntries(t)
	defer srcCleanup()

	// Create chunk manager from source
	manager, err := NewChunkManager(srcStor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	// Create destination storage
	dstStor, dstDir, dstCleanup := testutil.CreateEmptyTestStorage(t)
	defer dstCleanup()

	// Full transfer flow: Setup -> Write chunks -> Complete
	err = SetupTransfer(manager, dstStor)
	if err != nil {
		t.Fatalf("SetupTransfer failed: %v", err)
	}

	for i := 0; i < manager.GetChunkCount(); i++ {
		reader := manager.GetChunkReader(srcStor, i)
		_, err := manager.WriteChunk(dstStor, i, reader, 0, func(path string, size int64) {})
		reader.Close()
		if err != nil {
			t.Fatalf("WriteChunk %d failed: %v", i, err)
		}

		// Compute checksum after write
		_, err = manager.ComputeChunkChecksum(srcStor, i)
		if err != nil {
			t.Fatalf("ComputeChunkChecksum failed: %v", err)
		}
	}

	err = CompleteTransfer(manager, dstStor)
	if err != nil {
		t.Fatalf("CompleteTransfer failed: %v", err)
	}

	// Verify transfer using VerifyTransfer function
	isValid := VerifyTransfer(manager, dstStor)
	if !isValid {
		t.Error("VerifyTransfer should succeed after full transfer")
	}

	// Additionally verify by comparing source and destination files byte-by-byte
	for fileName, expectedContent := range expectedFiles {
		srcPath := filepath.Join(srcDir, fileName)
		dstPath := filepath.Join(dstDir, fileName)

		srcContent, err := os.ReadFile(srcPath)
		if err != nil {
			t.Errorf("Failed to read source file '%s': %v", fileName, err)
			continue
		}

		dstContent, err := os.ReadFile(dstPath)
		if err != nil {
			t.Errorf("Failed to read destination file '%s': %v", fileName, err)
			continue
		}

		if !bytes.Equal(srcContent, dstContent) {
			t.Errorf("File '%s' content differs between source and destination", fileName)
		}

		if !bytes.Equal(srcContent, expectedContent) {
			t.Errorf("File '%s' content doesn't match expected content", fileName)
		}
	}

	// Verify symlink target matches
	srcSymlinkTarget, err := os.Readlink(filepath.Join(srcDir, "link.txt"))
	if err != nil {
		t.Fatalf("Failed to read source symlink: %v", err)
	}
	dstSymlinkTarget, err := os.Readlink(filepath.Join(dstDir, "link.txt"))
	if err != nil {
		t.Fatalf("Failed to read destination symlink: %v", err)
	}
	if srcSymlinkTarget != dstSymlinkTarget {
		t.Errorf("Symlink target mismatch: source='%s', destination='%s'", srcSymlinkTarget, dstSymlinkTarget)
	}
}

func TestVerifyTransfer_FileMissing(t *testing.T) {
	srcStor, _, srcCleanup := testutil.CreateTestStorageWithEntries(t)
	defer srcCleanup()

	// Create chunk manager from source
	manager, err := NewChunkManager(srcStor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	// Compute checksums
	for i := 0; i < manager.GetChunkCount(); i++ {
		_, err := manager.ComputeChunkChecksum(srcStor, i)
		if err != nil {
			t.Fatalf("ComputeChunkChecksum failed: %v", err)
		}
	}

	// Create empty destination storage (files missing)
	dstStor, _, dstCleanup := testutil.CreateEmptyTestStorage(t)
	defer dstCleanup()

	// Verify should fail because files are missing
	isValid := VerifyTransfer(manager, dstStor)
	if isValid {
		t.Error("VerifyTransfer should fail when files are missing")
	}
}

// Test chunkReadCloser double close safety
func TestChunkReader_DoubleClose(t *testing.T) {
	stor, _, cleanup := testutil.CreateTestStorage(t, testutil.TestStorageOptions{
		Files: map[string][]byte{"small.txt": []byte("hello")},
	})
	defer cleanup()

	manager, err := NewChunkManager(stor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	// Read chunk multiple times to test Close behavior
	for i := 0; i < 2; i++ {
		reader := manager.GetChunkReader(stor, 0)
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Read %d failed: %v", i, err)
		}
		if len(data) == 0 {
			t.Errorf("Read %d returned empty data", i)
		}
		if err := reader.Close(); err != nil {
			t.Errorf("Close %d failed: %v", i, err)
		}
		// Close again should be safe
		if err := reader.Close(); err != nil {
			t.Errorf("Double close %d failed: %v", i, err)
		}
	}
}

func TestChunkReader_MultipleFileRanges(t *testing.T) {
	// Create multiple small files that will be in the same chunk
	stor, tmpDir, cleanup := testutil.CreateTestStorage(t, testutil.TestStorageOptions{
		Files: map[string][]byte{
			"a.txt": []byte("aaaa"),
			"b.txt": []byte("bbbb"),
			"c.txt": []byte("cccc"),
		},
	})
	defer cleanup()

	// Use large chunk size so all files are in one chunk
	manager, err := NewChunkManager(stor, nil, 0, 10000, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	if manager.GetChunkCount() != 1 {
		t.Fatalf("Expected 1 chunk, got %d", manager.GetChunkCount())
	}

	reader := manager.GetChunkReader(stor, 0)
	data, err := io.ReadAll(reader)
	reader.Close()

	if err != nil {
		t.Fatalf("Failed to read chunk: %v", err)
	}

	expectedLen := int64(12) // 4 + 4 + 4
	if int64(len(data)) != expectedLen {
		t.Errorf("Expected %d bytes, got %d", expectedLen, len(data))
	}

	// Verify the chunk contains all file contents in manifest order
	// Build expected content based on manifest file order
	var expectedContent []byte
	for i := range manager.manifest.Files {
		file := &manager.manifest.Files[i]
		fileContent, err := os.ReadFile(filepath.Join(tmpDir, file.Name))
		if err != nil {
			t.Fatalf("Failed to read file '%s': %v", file.Name, err)
		}
		expectedContent = append(expectedContent, fileContent...)
	}

	if !bytes.Equal(data, expectedContent) {
		t.Errorf("Chunk content mismatch:\n  expected: %q\n  actual:   %q", expectedContent, data)
	}
}

// Test UpdateChunkStatus failure case
func TestChunkManager_UpdateChunkStatus_Failure(t *testing.T) {
	stor, _, cleanup := testutil.CreateTestStorage(t, testutil.TestStorageOptions{
		Files: map[string][]byte{"test.txt": []byte("test")},
	})
	defer cleanup()

	manager, err := NewChunkManager(stor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	// Try to update with wrong old status
	success := manager.UpdateChunkStatus(0, types.ChunkStatusDownloading, types.ChunkStatusDone)
	if success {
		t.Error("UpdateChunkStatus should fail when old status doesn't match")
	}
}
