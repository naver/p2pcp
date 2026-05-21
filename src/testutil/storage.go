// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package testutil

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/naver/p2pcp/storage"
)

// TestStorageOptions configures test storage creation
type TestStorageOptions struct {
	// Files maps filename to content
	Files map[string][]byte
	// EmptyFiles is a list of empty file names to create
	EmptyFiles []string
	// Dirs is a list of directory names to create
	Dirs []string
	// Symlinks maps symlink name to target
	Symlinks map[string]string
}

// DefaultTestStorageOptions returns common test storage options
func DefaultTestStorageOptions() TestStorageOptions {
	return TestStorageOptions{
		Files: map[string][]byte{
			"file1.txt": bytes.Repeat([]byte("a"), 1000),
			"file2.txt": bytes.Repeat([]byte("b"), 2000),
			"file3.txt": bytes.Repeat([]byte("c"), 500),
		},
	}
}

// WithEntriesTestStorageOptions returns options with various entry types
func WithEntriesTestStorageOptions() TestStorageOptions {
	return TestStorageOptions{
		Files: map[string][]byte{
			"file1.txt":        bytes.Repeat([]byte("a"), 1000),
			"file2.txt":        bytes.Repeat([]byte("b"), 2000),
			"subdir/file3.txt": bytes.Repeat([]byte("c"), 500),
		},
		EmptyFiles: []string{"empty.txt"},
		Dirs:       []string{"subdir"},
		Symlinks:   map[string]string{"link.txt": "file1.txt"},
	}
}

// CreateTestStorage creates a test storage with specified options
// Returns storage, base directory path, and cleanup function
func CreateTestStorage(t *testing.T, opts TestStorageOptions) (storage.Storage, string, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "p2pcp-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	// Create directories first
	for _, dir := range opts.Dirs {
		dirPath := filepath.Join(tmpDir, dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			cleanup()
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	// Create files
	for name, content := range opts.Files {
		filePath := filepath.Join(tmpDir, name)
		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			cleanup()
			t.Fatalf("Failed to create parent dir for %s: %v", name, err)
		}
		if err := os.WriteFile(filePath, content, 0644); err != nil {
			cleanup()
			t.Fatalf("Failed to create test file %s: %v", name, err)
		}
	}

	// Create empty files
	for _, name := range opts.EmptyFiles {
		filePath := filepath.Join(tmpDir, name)
		if err := os.WriteFile(filePath, []byte{}, 0644); err != nil {
			cleanup()
			t.Fatalf("Failed to create empty file %s: %v", name, err)
		}
	}

	// Create symlinks
	for name, target := range opts.Symlinks {
		symlinkPath := filepath.Join(tmpDir, name)
		if err := os.Symlink(target, symlinkPath); err != nil {
			cleanup()
			t.Fatalf("Failed to create symlink %s: %v", name, err)
		}
	}

	// Create storage
	factory, err := storage.NewFactory(tmpDir)
	if err != nil {
		cleanup()
		t.Fatalf("Failed to create storage factory: %v", err)
	}

	stor, err := factory.Create()
	if err != nil {
		cleanup()
		t.Fatalf("Failed to create storage: %v", err)
	}

	return stor, tmpDir, cleanup
}

// CreateTestStorageDefault creates a test storage with default options
func CreateTestStorageDefault(t *testing.T) (storage.Storage, string, func()) {
	t.Helper()
	return CreateTestStorage(t, DefaultTestStorageOptions())
}

// CreateTestStorageWithEntries creates a test storage with various entry types
func CreateTestStorageWithEntries(t *testing.T) (storage.Storage, string, func()) {
	t.Helper()
	return CreateTestStorage(t, WithEntriesTestStorageOptions())
}

// CreateEmptyTestStorage creates an empty test storage
func CreateEmptyTestStorage(t *testing.T) (storage.Storage, string, func()) {
	t.Helper()
	return CreateTestStorage(t, TestStorageOptions{})
}
