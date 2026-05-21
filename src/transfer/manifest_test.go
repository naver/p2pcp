// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package transfer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/naver/p2pcp/peers"
	"github.com/naver/p2pcp/testutil"
	"github.com/naver/p2pcp/types"
	"github.com/naver/p2pcp/version"
)

func TestManifest_ComputeChecksum(t *testing.T) {
	stor, _, cleanup := testutil.CreateTestStorage(t, testutil.TestStorageOptions{
		Files: map[string][]byte{
			"file1.txt": []byte("hello"),
			"file2.txt": []byte("world"),
		},
	})
	defer cleanup()

	manager, err := NewChunkManager(stor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	// Checksum should be non-zero
	checksum := manager.GetManifestChecksum()
	if checksum == 0 {
		t.Error("Expected non-zero checksum")
	}

	// Creating same manifest should produce same checksum
	manager2, err := NewChunkManager(stor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	checksum2 := manager2.GetManifestChecksum()
	if checksum != checksum2 {
		t.Errorf("Expected same checksum for same files, got %d and %d", checksum, checksum2)
	}
}

func TestManifest_DifferentChunkSizeProducesDifferentChecksum(t *testing.T) {
	stor, _, cleanup := testutil.CreateTestStorage(t, testutil.TestStorageOptions{
		Files: map[string][]byte{
			"file1.txt": make([]byte, 2000),
			"file2.txt": make([]byte, 2000),
		},
	})
	defer cleanup()

	// Different chunk sizes should produce different checksums
	manager1, err := NewChunkManager(stor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	manager2, err := NewChunkManager(stor, nil, 0, 512, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	if manager1.GetManifestChecksum() == manager2.GetManifestChecksum() {
		t.Error("Different chunk sizes should produce different checksums")
	}
}

func TestNewManifest_WithStorage(t *testing.T) {
	stor, _, cleanup := testutil.CreateTestStorage(t, testutil.TestStorageOptions{
		Files: map[string][]byte{"test.txt": []byte("hello")},
	})
	defer cleanup()

	manifest, chunks, err := newManifest(stor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("newManifest failed: %v", err)
	}

	if manifest.ChunkSize != 1024 {
		t.Errorf("Expected ChunkSize 1024, got %d", manifest.ChunkSize)
	}
	if manifest.ChunkChecksum == 0 {
		t.Error("Expected non-zero ChunkChecksum")
	}
	if len(chunks) != 1 {
		t.Errorf("Expected 1 chunk, got %d", len(chunks))
	}
}

func TestNewManifest_RequiresStorageOrPeerList(t *testing.T) {
	// Neither storage nor peer provider - should fail
	_, _, err := newManifest(nil, nil, 0, 1024, 10)
	if err == nil {
		t.Error("Expected error when neither storage nor peer list provided")
	}
	if !strings.Contains(err.Error(), "at least one of local source directory or peer list required") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestNewManifest_WithPeerProvider(t *testing.T) {
	stor, _, cleanup := testutil.CreateTestStorage(t, testutil.TestStorageOptions{
		Files: map[string][]byte{"test.txt": []byte("hello")},
	})
	defer cleanup()

	sourceManager, err := NewChunkManager(stor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(sourceManager.GetManifestJson())
	}))
	defer server.Close()

	peerHost := strings.TrimPrefix(server.URL, "http://")
	provider, err := peers.NewStaticProvider(peerHost, 8080)
	if err != nil {
		t.Fatalf("Failed to create static provider: %v", err)
	}

	manifest, chunks, err := newManifest(nil, provider, 5*time.Second, 1024, 10)
	if err != nil {
		t.Fatalf("newManifest failed: %v", err)
	}

	if manifest.ChunkSize != 1024 {
		t.Errorf("Expected ChunkSize 1024, got %d", manifest.ChunkSize)
	}
	if manifest.ChunkChecksum == 0 {
		t.Error("Expected non-zero ChunkChecksum")
	}
	if len(chunks) != 1 {
		t.Errorf("Expected 1 chunk, got %d", len(chunks))
	}
}

func TestManifest_GetManifestFromPeer_VersionMismatch(t *testing.T) {
	// Create manifest with different version
	wrongManifest := &Manifest{
		MajorVersion:       "999",
		MinorVersion:       "0",
		ChunkSize:          1024,
		ChunkChecksum:      12345,
		TransferChunkCount: 1,
		Files:              []types.FileEntry{},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(wrongManifest)
	}))
	defer server.Close()

	manifest := &Manifest{
		MajorVersion: version.MajorVersion,
		MinorVersion: version.MinorVersion,
		ChunkSize:    1024,
	}

	peerHost := strings.TrimPrefix(server.URL, "http://")
	_, err := manifest.getManifestFromPeer(peerHost, 10)
	if err == nil {
		t.Error("Expected error for version mismatch")
	}
	if !strings.Contains(err.Error(), "different major version") {
		t.Errorf("Expected version mismatch error, got: %v", err)
	}
}

func TestManifest_GetManifestFromPeer_ChunkSizeMismatch(t *testing.T) {
	// Create manifest with different chunk size
	wrongManifest := &Manifest{
		MajorVersion:       version.MajorVersion,
		MinorVersion:       version.MinorVersion,
		ChunkSize:          512, // Different chunk size
		ChunkChecksum:      12345,
		TransferChunkCount: 1,
		Files:              []types.FileEntry{},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(wrongManifest)
	}))
	defer server.Close()

	manifest := &Manifest{
		MajorVersion: version.MajorVersion,
		MinorVersion: version.MinorVersion,
		ChunkSize:    1024,
	}

	peerHost := strings.TrimPrefix(server.URL, "http://")
	_, err := manifest.getManifestFromPeer(peerHost, 10)
	if err == nil {
		t.Error("Expected error for chunk size mismatch")
	}
	if !strings.Contains(err.Error(), "different chunk size") {
		t.Errorf("Expected chunk size mismatch error, got: %v", err)
	}
}

func TestManifest_GetManifestFromPeer_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	manifest := &Manifest{
		MajorVersion: version.MajorVersion,
		MinorVersion: version.MinorVersion,
		ChunkSize:    1024,
	}

	peerHost := strings.TrimPrefix(server.URL, "http://")
	_, err := manifest.getManifestFromPeer(peerHost, 10)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestManifest_GetManifestFromPeerList_Timeout(t *testing.T) {
	// Create a provider that returns unreachable peer
	provider, err := peers.NewStaticProvider("127.0.0.1:19999", 8080)
	if err != nil {
		t.Fatalf("Failed to create static provider: %v", err)
	}

	manifest := &Manifest{
		MajorVersion: version.MajorVersion,
		MinorVersion: version.MinorVersion,
		ChunkSize:    1024,
	}

	// Short timeout
	_, err = manifest.getManifestFromPeerList(provider, 100*time.Millisecond, 10)
	if err == nil {
		t.Error("Expected timeout error")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}

func TestManifest_GetManifestFromStorage_AllEntryTypes(t *testing.T) {
	fileContent := []byte("test content 12345")
	stor, _, cleanup := testutil.CreateTestStorage(t, testutil.TestStorageOptions{
		Files:      map[string][]byte{"file.txt": fileContent},
		EmptyFiles: []string{"empty.txt"},
		Dirs:       []string{"subdir"},
		Symlinks:   map[string]string{"link.txt": "file.txt"},
	})
	defer cleanup()

	manager, err := NewChunkManager(stor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("NewChunkManager failed: %v", err)
	}

	// Verify directory entry
	if len(manager.manifest.Dirs) != 1 {
		t.Fatalf("Expected exactly 1 directory, got %d", len(manager.manifest.Dirs))
	}
	dir := manager.manifest.Dirs[0]
	if dir.Name != "subdir" {
		t.Errorf("Expected directory name 'subdir', got '%s'", dir.Name)
	}
	if dir.Perm != 0755 {
		t.Errorf("Expected directory perm 0755, got %o", dir.Perm)
	}

	// Verify file entry
	if len(manager.manifest.Files) != 1 {
		t.Fatalf("Expected exactly 1 file, got %d", len(manager.manifest.Files))
	}
	file := &manager.manifest.Files[0]
	if file.Name != "file.txt" {
		t.Errorf("Expected file name 'file.txt', got '%s'", file.Name)
	}
	if file.Size != int64(len(fileContent)) {
		t.Errorf("Expected file size %d, got %d", len(fileContent), file.Size)
	}
	if file.Perm != 0644 {
		t.Errorf("Expected file perm 0644, got %o", file.Perm)
	}

	// Verify empty file entry
	if len(manager.manifest.EmptyFiles) != 1 {
		t.Fatalf("Expected exactly 1 empty file, got %d", len(manager.manifest.EmptyFiles))
	}
	emptyFile := manager.manifest.EmptyFiles[0]
	if emptyFile.Name != "empty.txt" {
		t.Errorf("Expected empty file name 'empty.txt', got '%s'", emptyFile.Name)
	}
	if emptyFile.Perm != 0644 {
		t.Errorf("Expected empty file perm 0644, got %o", emptyFile.Perm)
	}

	// Verify symlink entry
	if len(manager.manifest.Symlinks) != 1 {
		t.Fatalf("Expected exactly 1 symlink, got %d", len(manager.manifest.Symlinks))
	}
	symlink := manager.manifest.Symlinks[0]
	if symlink.Name != "link.txt" {
		t.Errorf("Expected symlink name 'link.txt', got '%s'", symlink.Name)
	}
	if symlink.SymlinkTo != "file.txt" {
		t.Errorf("Expected symlink target 'file.txt', got '%s'", symlink.SymlinkTo)
	}
}
