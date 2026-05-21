// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/naver/p2pcp/storage"
	"github.com/naver/p2pcp/testutil"
	"github.com/naver/p2pcp/transfer"
	"github.com/naver/p2pcp/types"
	"github.com/naver/p2pcp/utils"
)

func createTestMainContextForDownloaderTest(t *testing.T, srcStor, dstStor storage.Storage, chunkManager *transfer.ChunkManager) *MainContext {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	mainContext := &MainContext{
		UUID:           "test-uuid-12345",
		Src:            srcStor,
		Dst:            dstStor,
		ChunkManager:   chunkManager,
		DownloadCtx:    ctx,
		DownloadCancel: cancel,
		Statistics:     NewStatistics(false),
		Command:        utils.NewCommand("", "", 10*time.Second),
	}

	return mainContext
}

// MockDownloader implements Downloader interface for testing
type MockDownloader struct {
	name         string
	queue        *transfer.ChunkQueue
	chunkManager *transfer.ChunkManager
	storage      storage.Storage
	isValid      bool
	finCalled    bool
}

func NewMockDownloader(name string, chunkManager *transfer.ChunkManager, stor storage.Storage) *MockDownloader {
	downloader := &MockDownloader{
		name:         name,
		queue:        transfer.NewChunkQueue(),
		chunkManager: chunkManager,
		storage:      stor,
		isValid:      true,
	}

	// Populate queue with chunk indices
	indices := make([]int, chunkManager.GetChunkCount())
	for i := range chunkManager.GetChunkCount() {
		indices[i] = i
	}
	downloader.queue.PushShuffledList(indices)

	return downloader
}

func (d *MockDownloader) Name() string {
	return d.name
}

func (d *MockDownloader) GetQueue() *transfer.ChunkQueue {
	return d.queue
}

func (d *MockDownloader) RequestDownload(chunkIndex int, withChecksum bool) (uint64, io.ReadCloser, error) {
	reader := d.chunkManager.GetChunkReader(d.storage, chunkIndex)
	if withChecksum {
		checksum, err := d.chunkManager.ComputeChunkChecksum(d.storage, chunkIndex)
		if err != nil {
			return 0, nil, err
		}
		return checksum, reader, nil
	}
	return 0, reader, nil
}

func (d *MockDownloader) IsValid() bool {
	return d.isValid
}

func (d *MockDownloader) Fin() {
	d.finCalled = true
	d.isValid = false
}

func TestDownloaderInterface(t *testing.T) {
	srcStor, _, srcCleanup := testutil.CreateTestStorageDefault(t)
	defer srcCleanup()

	chunkManager, err := transfer.NewChunkManager(srcStor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("Failed to create chunk manager: %v", err)
	}

	mockDownloader := NewMockDownloader("Test", chunkManager, srcStor)

	// Verify MockDownloader implements Downloader interface
	var _ Downloader = mockDownloader

	if mockDownloader.Name() != "Test" {
		t.Errorf("Expected name 'Test', got '%s'", mockDownloader.Name())
	}

	if mockDownloader.GetQueue() == nil {
		t.Error("GetQueue should not return nil")
	}

	if !mockDownloader.IsValid() {
		t.Error("IsValid should be true initially")
	}

	mockDownloader.Fin()

	if !mockDownloader.finCalled {
		t.Error("Fin should have been called")
	}

	if mockDownloader.IsValid() {
		t.Error("IsValid should be false after Fin")
	}
}

func TestMockDownloader_RequestDownload(t *testing.T) {
	srcStor, _, srcCleanup := testutil.CreateTestStorageDefault(t)
	defer srcCleanup()

	chunkManager, err := transfer.NewChunkManager(srcStor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("Failed to create chunk manager: %v", err)
	}

	mockDownloader := NewMockDownloader("Test", chunkManager, srcStor)

	t.Run("without checksum", func(t *testing.T) {
		checksum, reader, err := mockDownloader.RequestDownload(0, false)
		if err != nil {
			t.Fatalf("RequestDownload failed: %v", err)
		}
		defer reader.Close()

		if checksum != 0 {
			t.Errorf("Expected checksum 0, got %d", checksum)
		}

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Failed to read: %v", err)
		}

		if int64(len(data)) != chunkManager.GetChunkSize(0) {
			t.Errorf("Expected %d bytes, got %d", chunkManager.GetChunkSize(0), len(data))
		}
	})

	t.Run("with checksum", func(t *testing.T) {
		checksum, reader, err := mockDownloader.RequestDownload(0, true)
		if err != nil {
			t.Fatalf("RequestDownload failed: %v", err)
		}
		defer reader.Close()

		if checksum == 0 {
			t.Error("Expected non-zero checksum")
		}
	})
}

func TestExecDownloader_ChunkVariations(t *testing.T) {
	tests := []struct {
		name               string
		useEmptyStorage    bool  // true: empty storage, false: default test storage
		chunkSize          int64 // ChunkManager chunk size
		expectedChunkCount int   // expected chunk count (0: empty storage, 1+: has chunks)
	}{
		{
			name:               "EmptyChunks",
			useEmptyStorage:    true,
			chunkSize:          1024,
			expectedChunkCount: 0,
		},
		{
			name:               "SingleChunk",
			useEmptyStorage:    false,
			chunkSize:          1024 * 1024, // large chunk size to ensure single chunk
			expectedChunkCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var srcStor storage.Storage
			var srcCleanup func()

			if tt.useEmptyStorage {
				srcStor, _, srcCleanup = testutil.CreateEmptyTestStorage(t)
			} else {
				srcStor, _, srcCleanup = testutil.CreateTestStorageDefault(t)
			}
			defer srcCleanup()

			chunkManager, err := transfer.NewChunkManager(srcStor, nil, 0, tt.chunkSize, 100)
			if err != nil {
				t.Fatalf("Failed to create chunk manager: %v", err)
			}

			// Verify chunk count
			actualChunkCount := chunkManager.GetChunkCount()
			if actualChunkCount != tt.expectedChunkCount {
				t.Fatalf("Expected %d chunks, got %d", tt.expectedChunkCount, actualChunkCount)
			}

			// Create dst storage
			dstStor, _, dstCleanup := testutil.CreateEmptyTestStorage(t)
			defer dstCleanup()

			// SetupTransfer required if there are chunks
			if actualChunkCount > 0 {
				err = transfer.SetupTransfer(chunkManager, dstStor)
				if err != nil {
					t.Fatalf("SetupTransfer failed: %v", err)
				}
			}

			mainContext := createTestMainContextForDownloaderTest(t, srcStor, dstStor, chunkManager)
			defer mainContext.DownloadCancel()

			mockDownloader := NewMockDownloader("Test", chunkManager, srcStor)

			done := make(chan struct{})
			go func() {
				execDownloader(mainContext, mockDownloader, 1)
				close(done)
			}()

			select {
			case <-done:
				if !mainContext.Completed.Load() {
					t.Error("Expected Completed to be true")
				}
				actualChunksReceived := int(mainContext.Statistics.ChunksReceivedTotal.Load())
				if actualChunksReceived != tt.expectedChunkCount {
					t.Errorf("Expected %d chunks received, got %d", tt.expectedChunkCount, actualChunksReceived)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Timeout waiting for execDownloader to complete")
			}
		})
	}
}

func TestExecDownloader_InvalidDownloader(t *testing.T) {
	srcStor, _, srcCleanup := testutil.CreateTestStorageDefault(t)
	defer srcCleanup()

	chunkManager, err := transfer.NewChunkManager(srcStor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("Failed to create chunk manager: %v", err)
	}

	mainContext := createTestMainContextForDownloaderTest(t, srcStor, nil, chunkManager)
	defer mainContext.DownloadCancel()

	mockDownloader := NewMockDownloader("Test", chunkManager, srcStor)

	// Invalidate downloader immediately
	mockDownloader.isValid = false

	// execDownloader should exit quickly when downloader is invalid
	done := make(chan struct{})
	go func() {
		execDownloader(mainContext, mockDownloader, 1)
		close(done)
	}()

	select {
	case <-done:
		// Good - should exit quickly
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout: execDownloader should exit when downloader is invalid")
	}
}

func TestExecDownloader_CancelContext(t *testing.T) {
	srcStor, _, srcCleanup := testutil.CreateTestStorageDefault(t)
	defer srcCleanup()

	chunkManager, err := transfer.NewChunkManager(srcStor, nil, 0, 500, 1)
	if err != nil {
		t.Fatalf("Failed to create chunk manager: %v", err)
	}

	mainContext := createTestMainContextForDownloaderTest(t, srcStor, nil, chunkManager)

	mockDownloader := NewMockDownloader("Test", chunkManager, srcStor)

	// Cancel context before starting execDownloader to test cancellation behavior
	mainContext.DownloadCancel()

	done := make(chan struct{})
	go func() {
		execDownloader(mainContext, mockDownloader, 1)
		close(done)
	}()

	select {
	case <-done:
		// Good - should exit immediately when context is already cancelled
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout: execDownloader should exit when context is cancelled")
	}
}

func TestChunkStatusTransitions(t *testing.T) {
	srcStor, _, srcCleanup := testutil.CreateTestStorageDefault(t)
	defer srcCleanup()

	chunkManager, err := transfer.NewChunkManager(srcStor, nil, 0, 500, 1)
	if err != nil {
		t.Fatalf("Failed to create chunk manager: %v", err)
	}

	if chunkManager.GetChunkCount() < 2 {
		t.Skip("Need at least 2 chunks for this test")
	}

	// Test chunk status transitions
	chunkIndex := 0

	// Initial status should be Pending
	if chunkManager.GetChunkStatus(chunkIndex) != types.ChunkStatusPending {
		t.Error("Initial status should be Pending")
	}

	// Transition to Downloading
	success := chunkManager.UpdateChunkStatus(chunkIndex, types.ChunkStatusPending, types.ChunkStatusDownloading)
	if !success {
		t.Error("Transition to Downloading should succeed")
	}

	if chunkManager.GetChunkStatus(chunkIndex) != types.ChunkStatusDownloading {
		t.Error("Status should be Downloading after transition")
	}

	// Try to transition from Pending to Downloading again (should fail since it's already Downloading)
	success = chunkManager.UpdateChunkStatus(chunkIndex, types.ChunkStatusPending, types.ChunkStatusDownloading)
	if success {
		t.Error("Transition from wrong state should fail")
	}

	// Set to Done
	chunkManager.SetChunkStatusDone(chunkIndex, time.Now())
	if chunkManager.GetChunkStatus(chunkIndex) != types.ChunkStatusDone {
		t.Error("Status should be Done after SetChunkStatusDone")
	}

	// Set back to Pending
	chunkManager.SetChunkStatusPending(chunkIndex)
	if chunkManager.GetChunkStatus(chunkIndex) != types.ChunkStatusPending {
		t.Error("Status should be Pending after SetChunkStatusPending")
	}
}

func TestDownloadFiles_LocalOnly(t *testing.T) {
	tests := []struct {
		name             string
		verifyOnComplete bool
	}{
		{"WithoutVerification", false},
		{"WithVerification", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srcStor, srcDir, srcCleanup := testutil.CreateTestStorageDefault(t)
			defer srcCleanup()

			chunkManager, err := transfer.NewChunkManager(srcStor, nil, 0, 1024*1024, 100)
			if err != nil {
				t.Fatalf("Failed to create chunk manager: %v", err)
			}

			dstStor, dstDir, dstCleanup := testutil.CreateEmptyTestStorage(t)
			defer dstCleanup()

			mainContext := createTestMainContextForDownloaderTest(t, srcStor, dstStor, chunkManager)
			defer mainContext.DownloadCancel()

			// Set flags for local download
			oldLocalFlag := *flagLocalNumConcurrent
			*flagLocalNumConcurrent = 2
			defer func() { *flagLocalNumConcurrent = oldLocalFlag }()

			oldVerifyFlag := *flagVerifyOnComplete
			*flagVerifyOnComplete = tt.verifyOnComplete
			defer func() { *flagVerifyOnComplete = oldVerifyFlag }()

			oldIdleTimeout := *flagTransferIdleTimeout
			*flagTransferIdleTimeout = 0
			defer func() { *flagTransferIdleTimeout = oldIdleTimeout }()

			err = DownloadFiles(mainContext)
			if err != nil {
				t.Fatalf("DownloadFiles failed: %v", err)
			}

			if !mainContext.Completed.Load() {
				t.Error("Expected Completed to be true after DownloadFiles")
			}

			if mainContext.Statistics.BytesReceivedTotal.Load() == 0 {
				t.Error("Expected non-zero bytes received")
			}

			if mainContext.Statistics.Elapsed == 0 {
				t.Error("Expected non-zero elapsed time")
			}

			if mainContext.ExitCode.Load() != 0 {
				t.Errorf("Expected ExitCode 0, got %d", mainContext.ExitCode.Load())
			}

			// Verify actual file content matches source
			expectedFiles := testutil.DefaultTestStorageOptions().Files
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

				if len(srcContent) != len(expectedContent) {
					t.Errorf("Source file '%s' size mismatch: expected %d, got %d",
						fileName, len(expectedContent), len(srcContent))
				}

				if len(dstContent) != len(srcContent) {
					t.Errorf("File '%s' size mismatch: source %d bytes, destination %d bytes",
						fileName, len(srcContent), len(dstContent))
					continue
				}

				for i := range srcContent {
					if srcContent[i] != dstContent[i] {
						t.Errorf("File '%s' content mismatch at byte %d: source=%d, destination=%d",
							fileName, i, srcContent[i], dstContent[i])
						break
					}
				}
			}
		})
	}
}

// TestDownloadFiles_IdleTimeout tests idle timeout when no download progress is made.
// Scenario: Src=nil and PeerListProvider=nil, so no downloader runs.
// This simulates a real scenario where p2pcp is started without -src and no peers are available.
func TestDownloadFiles_IdleTimeout(t *testing.T) {
	// Create a storage just for ChunkManager to read file list
	srcStor, _, srcCleanup := testutil.CreateTestStorageDefault(t)
	defer srcCleanup()

	chunkManager, err := transfer.NewChunkManager(srcStor, nil, 0, 1024, 100)
	if err != nil {
		t.Fatalf("Failed to create chunk manager: %v", err)
	}

	dstStor, _, dstCleanup := testutil.CreateEmptyTestStorage(t)
	defer dstCleanup()

	// Src=nil: no LocalDownloader
	// PeerListProvider=nil (default): no PeerDownloader
	mainContext := createTestMainContextForDownloaderTest(t, nil, dstStor, chunkManager)
	defer mainContext.DownloadCancel()

	oldIdleTimeout := *flagTransferIdleTimeout
	*flagTransferIdleTimeout = 1 * time.Second
	defer func() { *flagTransferIdleTimeout = oldIdleTimeout }()

	// DownloadFiles should timeout since no downloader is running
	_ = DownloadFiles(mainContext)

	if mainContext.ExitCode.Load() != 1 {
		t.Errorf("Expected ExitCode 1 (timeout), got %d", mainContext.ExitCode.Load())
	}
}

func TestPeerDownloadersFinWithRemaining(t *testing.T) {
	srcStor, _, srcCleanup := testutil.CreateTestStorageDefault(t)
	defer srcCleanup()

	chunkManager, err := transfer.NewChunkManager(srcStor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("Failed to create chunk manager: %v", err)
	}

	mainContext := createTestMainContextForDownloaderTest(t, srcStor, nil, chunkManager)
	defer mainContext.DownloadCancel()

	peerDownloaders := NewPeerDownloaders(mainContext, 2)

	// Don't add any peers, just call Fin - should work without panic
	peerDownloaders.Fin()
}

func TestPeerDownloader_Fin_GoroutineTermination(t *testing.T) {
	srcStor, _, srcCleanup := testutil.CreateTestStorageDefault(t)
	defer srcCleanup()

	chunkManager, err := transfer.NewChunkManager(srcStor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("Failed to create chunk manager: %v", err)
	}

	mainContext := createTestMainContextForDownloaderTest(t, srcStor, nil, chunkManager)
	defer mainContext.DownloadCancel()

	// Create PeerDownloader directly (will fail HTTP requests but goroutine still runs)
	downloader := NewPeerDownloader(mainContext, "127.0.0.1:59999", 2)

	// Verify initial state
	if !downloader.IsValid() {
		t.Error("Expected IsValid() to be true before Fin()")
	}

	// Fin() should complete without blocking indefinitely
	done := make(chan struct{})
	go func() {
		downloader.Fin()
		close(done)
	}()

	select {
	case <-done:
		// Goroutine terminated successfully
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout: Fin() should terminate goroutine within timeout")
	}

	// After Fin(), IsValid() should return false
	if downloader.IsValid() {
		t.Error("Expected IsValid() to be false after Fin()")
	}
}

func TestPeerDownloaders_Fin_WithMultiplePeers(t *testing.T) {
	srcStor, _, srcCleanup := testutil.CreateTestStorageDefault(t)
	defer srcCleanup()

	chunkManager, err := transfer.NewChunkManager(srcStor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("Failed to create chunk manager: %v", err)
	}

	mainContext := createTestMainContextForDownloaderTest(t, srcStor, nil, chunkManager)

	peerDownloaders := NewPeerDownloaders(mainContext, 2)

	// Add multiple peer downloaders
	hosts := []string{"127.0.0.1:59991", "127.0.0.1:59992", "127.0.0.1:59993"}
	for _, host := range hosts {
		peerDownloaders.Add(host)
	}

	// Give goroutines time to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context to stop execDownloader goroutines
	mainContext.DownloadCancel()

	// Fin() should complete for all peers without blocking
	done := make(chan struct{})
	go func() {
		peerDownloaders.Fin()
		close(done)
	}()

	select {
	case <-done:
		// All goroutines terminated successfully
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout: Fin() should terminate all peer goroutines after context cancel")
	}
}

func TestStatisticsIntegration(t *testing.T) {
	srcStor, _, srcCleanup := testutil.CreateTestStorageDefault(t)
	defer srcCleanup()

	chunkManager, err := transfer.NewChunkManager(srcStor, nil, 0, 500, 1)
	if err != nil {
		t.Fatalf("Failed to create chunk manager: %v", err)
	}

	dstStor, _, dstCleanup := testutil.CreateEmptyTestStorage(t)
	defer dstCleanup()

	// Calculate expected total bytes from test storage options
	expectedFiles := testutil.DefaultTestStorageOptions().Files
	var expectedTotalBytes int64
	for _, content := range expectedFiles {
		expectedTotalBytes += int64(len(content))
	}

	// Use statistics enabled
	mainContext := createTestMainContextForDownloaderTest(t, srcStor, dstStor, chunkManager)
	mainContext.Statistics = NewStatistics(true) // Enable statistics
	defer mainContext.DownloadCancel()

	oldLocalFlag := *flagLocalNumConcurrent
	*flagLocalNumConcurrent = 2
	defer func() { *flagLocalNumConcurrent = oldLocalFlag }()

	oldVerifyFlag := *flagVerifyOnComplete
	*flagVerifyOnComplete = false
	defer func() { *flagVerifyOnComplete = oldVerifyFlag }()

	oldIdleTimeout := *flagTransferIdleTimeout
	*flagTransferIdleTimeout = 0
	defer func() { *flagTransferIdleTimeout = oldIdleTimeout }()

	err = DownloadFiles(mainContext)
	if err != nil {
		t.Fatalf("DownloadFiles failed: %v", err)
	}

	// Check statistics are populated
	stat := mainContext.Statistics

	if !stat.IsSetBytesReceivedTotal.Load() {
		t.Error("IsSetBytesReceivedTotal should be true")
	}

	// Verify BytesReceivedTotal matches expected total bytes
	actualBytes := stat.BytesReceivedTotal.Load()
	if actualBytes == 0 {
		t.Error("BytesReceivedTotal should be non-zero")
	}
	if actualBytes != expectedTotalBytes {
		t.Errorf("BytesReceivedTotal = %d, want %d", actualBytes, expectedTotalBytes)
	}

	// Verify ChunksReceivedTotal matches expected chunk count
	expectedChunks := int64(chunkManager.GetChunkCount())
	actualChunks := stat.ChunksReceivedTotal.Load()
	if actualChunks == 0 {
		t.Error("ChunksReceivedTotal should be non-zero")
	}
	if actualChunks != expectedChunks {
		t.Errorf("ChunksReceivedTotal = %d, want %d", actualChunks, expectedChunks)
	}

	if stat.Elapsed == 0 {
		t.Error("Elapsed should be non-zero")
	}

	// Data map should have Local downloader stats
	stat.Lock.RLock()
	localBytes, ok := stat.Data["[Local] received_bytes"]
	stat.Lock.RUnlock()

	if !ok {
		t.Error("Expected Local downloader stats in Data map")
	}

	if localBytes == "0" {
		t.Error("Local received_bytes should be non-zero")
	}
}

// LocalDownloader Tests

func TestLocalDownloader_NewLocalDownloader(t *testing.T) {
	srcStor, _, srcCleanup := testutil.CreateTestStorageDefault(t)
	defer srcCleanup()

	chunkManager, err := transfer.NewChunkManager(srcStor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("Failed to create chunk manager: %v", err)
	}

	downloader := NewLocalDownloader(chunkManager, srcStor)

	if downloader == nil {
		t.Fatal("Expected non-nil downloader")
	}

	if downloader.Name() != "Local" {
		t.Errorf("Expected name 'Local', got '%s'", downloader.Name())
	}

	if downloader.GetQueue() == nil {
		t.Error("Expected non-nil queue")
	}

	// Queue should be populated with all chunk indices
	expectedChunks := chunkManager.GetChunkCount()
	actualQueueSize := 0
	for {
		_, ok := downloader.GetQueue().Pop()
		if !ok {
			break
		}
		actualQueueSize++
	}

	if actualQueueSize != expectedChunks {
		t.Errorf("Expected queue to have %d chunks, got %d", expectedChunks, actualQueueSize)
	}
}

func TestLocalDownloader_IsValid(t *testing.T) {
	srcStor, _, srcCleanup := testutil.CreateTestStorageDefault(t)
	defer srcCleanup()

	chunkManager, err := transfer.NewChunkManager(srcStor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("Failed to create chunk manager: %v", err)
	}

	downloader := NewLocalDownloader(chunkManager, srcStor)

	// LocalDownloader.IsValid() always returns true
	if !downloader.IsValid() {
		t.Error("LocalDownloader.IsValid() should always return true")
	}
}

func TestLocalDownloader_Fin(t *testing.T) {
	srcStor, _, srcCleanup := testutil.CreateTestStorageDefault(t)
	defer srcCleanup()

	chunkManager, err := transfer.NewChunkManager(srcStor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("Failed to create chunk manager: %v", err)
	}

	downloader := NewLocalDownloader(chunkManager, srcStor)

	// Fin() should not panic and should be callable multiple times
	downloader.Fin()
	downloader.Fin() // Double call should be safe

	// After Fin(), IsValid should still return true (LocalDownloader has no invalidation logic)
	if !downloader.IsValid() {
		t.Error("LocalDownloader.IsValid() should still return true after Fin()")
	}
}

func TestLocalDownloader_RequestDownload_WithoutChecksum(t *testing.T) {
	srcStor, _, srcCleanup := testutil.CreateTestStorageDefault(t)
	defer srcCleanup()

	chunkManager, err := transfer.NewChunkManager(srcStor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("Failed to create chunk manager: %v", err)
	}

	downloader := NewLocalDownloader(chunkManager, srcStor)

	checksum, reader, err := downloader.RequestDownload(0, false)
	if err != nil {
		t.Fatalf("RequestDownload failed: %v", err)
	}
	defer reader.Close()

	if checksum != 0 {
		t.Errorf("Expected checksum 0 when withChecksum=false, got %d", checksum)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	expectedSize := chunkManager.GetChunkSize(0)
	if int64(len(data)) != expectedSize {
		t.Errorf("Expected %d bytes, got %d", expectedSize, len(data))
	}
}

func TestLocalDownloader_RequestDownload_WithChecksum(t *testing.T) {
	srcStor, _, srcCleanup := testutil.CreateTestStorageDefault(t)
	defer srcCleanup()

	chunkManager, err := transfer.NewChunkManager(srcStor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("Failed to create chunk manager: %v", err)
	}

	downloader := NewLocalDownloader(chunkManager, srcStor)

	checksum, reader, err := downloader.RequestDownload(0, true)
	if err != nil {
		t.Fatalf("RequestDownload failed: %v", err)
	}
	defer reader.Close()

	if checksum == 0 {
		t.Error("Expected non-zero checksum when withChecksum=true")
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	expectedSize := chunkManager.GetChunkSize(0)
	if int64(len(data)) != expectedSize {
		t.Errorf("Expected %d bytes, got %d", expectedSize, len(data))
	}

	// Verify checksum was cached in chunk manager
	cachedChecksum := chunkManager.GetChunkChecksum(0)
	if cachedChecksum != checksum {
		t.Errorf("Expected cached checksum %d, got %d", checksum, cachedChecksum)
	}
}

func TestLocalDownloader_ImplementsDownloaderInterface(t *testing.T) {
	srcStor, _, srcCleanup := testutil.CreateTestStorageDefault(t)
	defer srcCleanup()

	chunkManager, err := transfer.NewChunkManager(srcStor, nil, 0, 1024, 10)
	if err != nil {
		t.Fatalf("Failed to create chunk manager: %v", err)
	}

	downloader := NewLocalDownloader(chunkManager, srcStor)

	// Verify LocalDownloader implements Downloader interface
	var _ Downloader = downloader
}
