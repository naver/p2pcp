// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package storage

import (
	"os"
	"testing"
)

func TestNewFactory_EmptyURL(t *testing.T) {
	factory, err := NewFactory("")
	if err != nil {
		t.Fatalf("NewFactory failed: %v", err)
	}
	if factory.storageType != StorageTypeNone {
		t.Errorf("Expected storageType None, got %s", factory.storageType)
	}
	if factory.storagePath != "" {
		t.Errorf("Expected empty storagePath, got %s", factory.storagePath)
	}
}

func TestNewFactory_LocalWithPrefix(t *testing.T) {
	factory, err := NewFactory("local:/tmp/test")
	if err != nil {
		t.Fatalf("NewFactory failed: %v", err)
	}
	if factory.storageType != StorageTypeLocal {
		t.Errorf("Expected storageType Local, got %s", factory.storageType)
	}
	if factory.storagePath != "/tmp/test" {
		t.Errorf("Expected storagePath /tmp/test, got %s", factory.storagePath)
	}
}

func TestNewFactory_LocalWithoutPrefix(t *testing.T) {
	factory, err := NewFactory("/tmp/test")
	if err != nil {
		t.Fatalf("NewFactory failed: %v", err)
	}
	if factory.storageType != StorageTypeLocal {
		t.Errorf("Expected storageType Local, got %s", factory.storageType)
	}
	if factory.storagePath != "/tmp/test" {
		t.Errorf("Expected storagePath /tmp/test, got %s", factory.storagePath)
	}
}

func TestNewFactory_S3(t *testing.T) {
	factory, err := NewFactory("s3:mybucket/path")
	if err != nil {
		t.Fatalf("NewFactory failed: %v", err)
	}
	if factory.storageType != StorageTypeS3 {
		t.Errorf("Expected storageType S3, got %s", factory.storageType)
	}
}

func TestNewFactory_Appdepot(t *testing.T) {
	factory, err := NewFactory("appdepot:myapp/env")
	if err != nil {
		t.Fatalf("NewFactory failed: %v", err)
	}
	if factory.storageType != StorageTypeAppdepot {
		t.Errorf("Expected storageType Appdepot, got %s", factory.storageType)
	}
}

func TestNewFactory_InvalidType(t *testing.T) {
	_, err := NewFactory("invalid:path")
	if err == nil {
		t.Error("Expected error for invalid storage type")
	}
}

func TestNewFactory_CaseInsensitive(t *testing.T) {
	tests := []struct {
		url          string
		expectedType StorageType
	}{
		{"LOCAL:/tmp/test", StorageTypeLocal},
		{"Local:/tmp/test", StorageTypeLocal},
		{"S3:bucket", StorageTypeS3},
		{"APPDEPOT:app", StorageTypeAppdepot},
	}

	for _, tt := range tests {
		factory, err := NewFactory(tt.url)
		if err != nil {
			t.Errorf("NewFactory(%s) failed: %v", tt.url, err)
			continue
		}
		if factory.storageType != tt.expectedType {
			t.Errorf("NewFactory(%s): expected type %s, got %s", tt.url, tt.expectedType, factory.storageType)
		}
	}
}

func TestFactory_Create_None(t *testing.T) {
	factory, _ := NewFactory("")
	storage, err := factory.Create()
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if storage != nil {
		t.Error("Expected nil storage for StorageTypeNone")
	}
}

func TestFactory_Create_Local(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "p2pcp-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	factory, _ := NewFactory("local:" + tmpDir)
	storage, err := factory.Create()
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if storage == nil {
		t.Fatal("Expected non-nil storage")
	}
	if storage.GetBaseDirectory() != tmpDir {
		t.Errorf("Expected base directory %s, got %s", tmpDir, storage.GetBaseDirectory())
	}
}

func TestFactory_Create_LocalWithPathOption(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "p2pcp-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	factory, _ := NewFactory("local:/original/path")
	storage, err := factory.Create(WithPath(tmpDir))
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if storage == nil {
		t.Fatal("Expected non-nil storage")
	}
	if storage.GetBaseDirectory() != tmpDir {
		t.Errorf("Expected base directory %s (from option), got %s", tmpDir, storage.GetBaseDirectory())
	}
}

func TestFactory_Create_S3NotSupported(t *testing.T) {
	factory, _ := NewFactory("s3:bucket/path")
	_, err := factory.Create()
	if err == nil {
		t.Error("Expected error for unsupported S3 storage")
	}
}

func TestFactory_Create_AppdepotNotSupported(t *testing.T) {
	factory, _ := NewFactory("appdepot:app/env")
	_, err := factory.Create()
	if err == nil {
		t.Error("Expected error for unsupported Appdepot storage")
	}
}
