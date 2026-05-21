// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCommand_ImmediateStopNoDeadlock tests that Stop() can be called immediately
// after RunOnEachFileComplete() without causing a deadlock
func TestCommand_ImmediateStopNoDeadlock(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "p2pcp-cmd-nodeadlock-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputFile := filepath.Join(tmpDir, "nodeadlock.txt")

	cmdStr := "echo '" + TemplateFilePath + "' >> " + outputFile
	cmd := NewCommand("", cmdStr, 5*time.Second)

	cmd.Run()

	// Queue commands and immediately stop
	cmd.RunOnEachFileComplete("/test/file1.txt", 100)
	cmd.RunOnEachFileComplete("/test/file2.txt", 200)
	cmd.RunOnEachFileComplete("/test/file3.txt", 300)

	// Stop immediately without sleeping - this should NOT deadlock!
	// The fix ensures all queued commands are processed before Stop() returns
	done := make(chan bool, 1)
	go func() {
		cmd.Stop()
		done <- true
	}()

	// Wait for Stop to complete or timeout
	select {
	case <-done:
		// Success - Stop() completed without deadlock
		t.Log("Stop() completed successfully without deadlock")
	case <-time.After(5 * time.Second):
		t.Fatal("Deadlock detected! Stop() did not complete within 5 seconds")
	}

	// Verify that all queued commands were executed
	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	output := string(data)
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) < 3 {
		t.Errorf("Expected at least 3 commands to execute, got %d", len(lines))
	}

	// Verify all files are in output
	if !strings.Contains(output, "/test/file1.txt") {
		t.Error("Expected file1.txt in output")
	}
	if !strings.Contains(output, "/test/file2.txt") {
		t.Error("Expected file2.txt in output")
	}
	if !strings.Contains(output, "/test/file3.txt") {
		t.Error("Expected file3.txt in output")
	}
}

// TestCommand_QueueWhileRunning tests queuing commands while Run() is active
func TestCommand_QueueWhileRunning(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "p2pcp-cmd-queue-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputFile := filepath.Join(tmpDir, "queue.txt")

	cmdStr := "echo '" + TemplateFilePath + "' >> " + outputFile
	cmd := NewCommand("", cmdStr, 5*time.Second)

	cmd.Run()

	// Queue multiple commands rapidly
	for i := 0; i < 50; i++ {
		filePath := filepath.Join("/test", fmt.Sprintf("rapid_%d.txt", i))
		cmd.RunOnEachFileComplete(filePath, int64(i*100))
	}

	// Stop and verify all were executed
	cmd.Stop()

	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	output := string(data)
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) < 50 {
		t.Errorf("Expected 50 commands to execute, got %d", len(lines))
	}
}

// TestCommand_StopGuaranteesExecution verifies that once RunOnEachFileComplete
// is called, the command will be executed even if Stop() is called immediately
func TestCommand_StopGuaranteesExecution(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "p2pcp-cmd-guarantee-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputFile := filepath.Join(tmpDir, "guarantee.txt")

	// Use a command that takes some time to execute
	cmdStr := "sleep 0.05 && echo '" + TemplateFilePath + "' >> " + outputFile
	cmd := NewCommand("", cmdStr, 5*time.Second)

	cmd.Run()

	// Queue 5 commands
	for i := 0; i < 5; i++ {
		filePath := filepath.Join("/test", fmt.Sprintf("guarantee_%d.txt", i))
		cmd.RunOnEachFileComplete(filePath, int64(i*100))
	}

	// Stop should wait for all 5 commands to complete
	startTime := time.Now()
	cmd.Stop()
	elapsed := time.Since(startTime)

	// Stop should have waited for at least some commands to execute
	// (5 commands * 50ms sleep = at least 250ms if sequential, less if parallel)
	if elapsed < 100*time.Millisecond {
		t.Logf("Warning: Stop completed very quickly (%v), commands might not have executed", elapsed)
	}

	// Verify all 5 commands executed
	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	output := string(data)
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 5 {
		t.Errorf("Expected exactly 5 commands to execute, got %d", len(lines))
		t.Logf("Output:\n%s", output)
	}

	// Verify all expected files are in output
	for i := 0; i < 5; i++ {
		expectedFile := fmt.Sprintf("guarantee_%d.txt", i)
		if !strings.Contains(output, expectedFile) {
			t.Errorf("Expected %s in output", expectedFile)
		}
	}
}

// TestCommand_NoRaceCondition tests that WaitGroup.Add happens before channel send
func TestCommand_NoRaceCondition(t *testing.T) {
	// This test is mainly useful when run with -race flag
	tmpDir, err := os.MkdirTemp("", "p2pcp-cmd-race-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputFile := filepath.Join(tmpDir, "race.txt")
	cmdStr := "echo 'x' >> " + outputFile

	cmd := NewCommand("", cmdStr, 5*time.Second)
	cmd.Run()

	// Rapidly queue and stop to try to trigger race condition
	for i := 0; i < 100; i++ {
		cmd.RunOnEachFileComplete("/test/file.txt", 100)
	}

	// Stop should not race with the WaitGroup operations
	cmd.Stop()

	// Just verify it didn't crash or deadlock
	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 100 {
		t.Errorf("Expected 100 commands, got %d", len(lines))
	}
}

// TestCommand_ChannelBufferFull tests behavior when channel buffer is full
func TestCommand_ChannelBufferFull(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "p2pcp-cmd-buffer-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputFile := filepath.Join(tmpDir, "buffer.txt")

	// Use slow command to fill up the channel buffer
	cmdStr := "sleep 0.1 && echo '" + TemplateFilePath + "' >> " + outputFile
	cmd := NewCommand("", cmdStr, 5*time.Second)

	cmd.Run()

	// Queue many commands quickly (more than buffer size)
	// Channel buffer is size 1, so this will test blocking sends
	numCommands := 10
	for i := 0; i < numCommands; i++ {
		filePath := filepath.Join("/test", fmt.Sprintf("buffer_%d.txt", i))
		cmd.RunOnEachFileComplete(filePath, int64(i*100))
	}

	// Stop should not deadlock even if channel was full
	cmd.Stop()

	// Verify all commands were executed
	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	output := string(data)
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) < numCommands {
		t.Errorf("Expected %d commands to execute, got %d", numCommands, len(lines))
	}
}
