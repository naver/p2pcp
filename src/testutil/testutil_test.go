// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package testutil

import (
	"os"
	"testing"
)

func TestCreateTestStorageDefault(t *testing.T) {
	stor, dir, cleanup := CreateTestStorageDefault(t)
	defer cleanup()

	if stor == nil {
		t.Fatal("Expected non-nil storage")
	}

	if dir == "" {
		t.Fatal("Expected non-empty directory path")
	}

	// Verify directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatalf("Directory was not created: %v", err)
	}

	// Verify storage base directory matches
	if stor.GetBaseDirectory() != dir {
		t.Errorf("Storage base directory mismatch: expected %s, got %s", dir, stor.GetBaseDirectory())
	}

	// Verify default files were created
	defaultOpts := DefaultTestStorageOptions()
	for name := range defaultOpts.Files {
		filePath := dir + "/" + name
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Expected file %s was not created", name)
		}
	}
}

func TestCreateTestStorageWithEntries(t *testing.T) {
	stor, dir, cleanup := CreateTestStorageWithEntries(t)
	defer cleanup()

	if stor == nil {
		t.Fatal("Expected non-nil storage")
	}

	// Verify various entry types
	opts := WithEntriesTestStorageOptions()

	// Check directories
	for _, dirName := range opts.Dirs {
		dirPath := dir + "/" + dirName
		info, err := os.Stat(dirPath)
		if err != nil {
			t.Errorf("Directory %s was not created: %v", dirName, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("Expected %s to be a directory", dirName)
		}
	}

	// Check files
	for name := range opts.Files {
		filePath := dir + "/" + name
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("File %s was not created", name)
		}
	}

	// Check empty files
	for _, name := range opts.EmptyFiles {
		filePath := dir + "/" + name
		info, err := os.Stat(filePath)
		if err != nil {
			t.Errorf("Empty file %s was not created: %v", name, err)
			continue
		}
		if info.Size() != 0 {
			t.Errorf("Empty file %s should have size 0, got %d", name, info.Size())
		}
	}

	// Check symlinks
	for name, target := range opts.Symlinks {
		symlinkPath := dir + "/" + name
		actualTarget, err := os.Readlink(symlinkPath)
		if err != nil {
			t.Errorf("Symlink %s was not created: %v", name, err)
			continue
		}
		if actualTarget != target {
			t.Errorf("Symlink %s target mismatch: expected %s, got %s", name, target, actualTarget)
		}
	}
}

func TestCreateEmptyTestStorage(t *testing.T) {
	stor, dir, cleanup := CreateEmptyTestStorage(t)
	defer cleanup()

	if stor == nil {
		t.Fatal("Expected non-nil storage")
	}

	// Verify directory exists but is empty
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("Expected empty directory, got %d entries", len(entries))
	}
}

func TestCreateTestStorage_CustomOptions(t *testing.T) {
	opts := TestStorageOptions{
		Files: map[string][]byte{
			"custom.txt": []byte("custom content"),
		},
		Dirs:       []string{"custom_dir"},
		EmptyFiles: []string{"custom_empty.txt"},
		Symlinks:   map[string]string{"custom_link": "custom.txt"},
	}

	stor, dir, cleanup := CreateTestStorage(t, opts)
	defer cleanup()

	if stor == nil {
		t.Fatal("Expected non-nil storage")
	}

	// Verify custom file
	content, err := os.ReadFile(dir + "/custom.txt")
	if err != nil {
		t.Fatalf("Failed to read custom file: %v", err)
	}
	if string(content) != "custom content" {
		t.Errorf("Custom file content mismatch: expected 'custom content', got '%s'", string(content))
	}

	// Verify custom directory
	info, err := os.Stat(dir + "/custom_dir")
	if err != nil {
		t.Fatalf("Custom directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Expected custom_dir to be a directory")
	}

	// Verify custom empty file
	info, err = os.Stat(dir + "/custom_empty.txt")
	if err != nil {
		t.Fatalf("Custom empty file was not created: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("Custom empty file should have size 0, got %d", info.Size())
	}

	// Verify custom symlink
	target, err := os.Readlink(dir + "/custom_link")
	if err != nil {
		t.Fatalf("Custom symlink was not created: %v", err)
	}
	if target != "custom.txt" {
		t.Errorf("Custom symlink target mismatch: expected 'custom.txt', got '%s'", target)
	}
}
