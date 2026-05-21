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

func TestNewCommand(t *testing.T) {
	t.Run("with empty commands", func(t *testing.T) {
		cmd := NewCommand("", "", 5*time.Second)
		if cmd == nil {
			t.Fatal("Expected non-nil command")
		}
		if cmd.commandTimeout != 5*time.Second {
			t.Errorf("Expected timeout 5s, got %v", cmd.commandTimeout)
		}
	})

	t.Run("with on complete command", func(t *testing.T) {
		cmd := NewCommand("echo 'complete'", "", 5*time.Second)
		if cmd == nil {
			t.Fatal("Expected non-nil command")
		}
	})

	t.Run("with on each file complete command", func(t *testing.T) {
		cmd := NewCommand("", "echo 'file complete'", 5*time.Second)
		if cmd == nil {
			t.Fatal("Expected non-nil command")
		}
	})
}

func TestCommand_TemplateReplacement(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "p2pcp-cmd-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputFile := filepath.Join(tmpDir, "output.txt")

	t.Run("on complete template replacement", func(t *testing.T) {
		cmdStr := "echo 'Files:" + TemplateFileCount + " Size:" + TemplateTotalFileSize + " Time:" + TemplateElapsedTime + "' >> " + outputFile
		cmd := NewCommand(cmdStr, "", 5*time.Second)

		cmd.Run()

		cmd.RunOnComplete(42, 1024000, "10s")

		cmd.Stop()

		data, err := os.ReadFile(outputFile)
		if err != nil {
			t.Fatalf("Failed to read output file: %v", err)
		}

		output := string(data)
		if !strings.Contains(output, "Files:42") {
			t.Errorf("Expected 'Files:42' in output, got: %s", output)
		}
		if !strings.Contains(output, "Size:1024000") {
			t.Errorf("Expected 'Size:1024000' in output, got: %s", output)
		}
		if !strings.Contains(output, "Time:10s") {
			t.Errorf("Expected 'Time:10s' in output, got: %s", output)
		}
	})

	t.Run("on each file complete template replacement", func(t *testing.T) {
		os.Remove(outputFile)

		cmdStr := "echo 'File:" + TemplateFilePath + " Size:" + TemplateFileSize + "' >> " + outputFile
		cmd := NewCommand("", cmdStr, 5*time.Second)

		cmd.Run()

		cmd.RunOnEachFileComplete("/path/to/file1.txt", 512)
		cmd.RunOnEachFileComplete("/path/to/file2.txt", 1024)

		cmd.Stop()

		data, err := os.ReadFile(outputFile)
		if err != nil {
			t.Fatalf("Failed to read output file: %v", err)
		}

		output := string(data)
		if !strings.Contains(output, "File:/path/to/file1.txt") {
			t.Errorf("Expected 'File:/path/to/file1.txt' in output, got: %s", output)
		}
		if !strings.Contains(output, "Size:512") {
			t.Errorf("Expected 'Size:512' in output")
		}
		if !strings.Contains(output, "File:/path/to/file2.txt") {
			t.Errorf("Expected 'File:/path/to/file2.txt' in output")
		}
		if !strings.Contains(output, "Size:1024") {
			t.Errorf("Expected 'Size:1024' in output")
		}
	})
}

func TestCommand_ParallelExecution(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "p2pcp-cmd-parallel-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputFile := filepath.Join(tmpDir, "parallel.txt")

	cmdStr := "echo '" + TemplateFilePath + "' >> " + outputFile
	cmd := NewCommand("", cmdStr, 5*time.Second)

	cmd.Run()

	fileCount := 20
	for i := 0; i < fileCount; i++ {
		filePath := filepath.Join("/test", fmt.Sprintf("file_%d.txt", i))
		cmd.RunOnEachFileComplete(filePath, int64(i*100))
	}

	cmd.Stop()

	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	output := string(data)
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) < fileCount {
		t.Errorf("Expected at least %d lines, got %d", fileCount, len(lines))
	}
}

func TestCommand_ImmediateStop(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "p2pcp-cmd-stop-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputFile := filepath.Join(tmpDir, "stop.txt")

	cmdStr := "sleep 0.05 && echo '" + TemplateFilePath + "' >> " + outputFile
	cmd := NewCommand("", cmdStr, 5*time.Second)

	cmd.Run()

	for i := 0; i < 10; i++ {
		filePath := filepath.Join("/test", fmt.Sprintf("file_%d.txt", i))
		cmd.RunOnEachFileComplete(filePath, int64(i*100))
	}

	// Stop waits for all queued commands
	cmd.Stop()

	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	output := string(data)
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) < 10 {
		t.Errorf("Expected 10 completed commands, got %d", len(lines))
	}
}

func TestCommand_Timeout(t *testing.T) {
	tests := []struct {
		name           string
		sleepDuration  time.Duration
		timeout        time.Duration
		shouldComplete bool
	}{
		{
			name:           "command completes well before timeout",
			sleepDuration:  50 * time.Millisecond,
			timeout:        500 * time.Millisecond,
			shouldComplete: true,
		},
		{
			name:           "command completes just before timeout",
			sleepDuration:  100 * time.Millisecond,
			timeout:        300 * time.Millisecond,
			shouldComplete: true,
		},
		{
			name:           "command exceeds timeout slightly",
			sleepDuration:  500 * time.Millisecond,
			timeout:        100 * time.Millisecond,
			shouldComplete: false,
		},
		{
			name:           "command exceeds timeout significantly",
			sleepDuration:  2 * time.Second,
			timeout:        100 * time.Millisecond,
			shouldComplete: false,
		},
		{
			name:           "long timeout with quick command",
			sleepDuration:  50 * time.Millisecond,
			timeout:        3 * time.Second,
			shouldComplete: true,
		},
		{
			name:           "short timeout with long command",
			sleepDuration:  3 * time.Second,
			timeout:        50 * time.Millisecond,
			shouldComplete: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "p2pcp-cmd-timeout-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			outputFile := filepath.Join(tmpDir, "timeout.txt")
			marker := "completed"

			sleepSec := tt.sleepDuration.Seconds()
			cmdStr := fmt.Sprintf("sleep %v && echo '%s' >> %s", sleepSec, marker, outputFile)
			cmd := NewCommand("", cmdStr, tt.timeout)

			cmd.Run()

			startTime := time.Now()
			cmd.RunOnEachFileComplete("/test/file.txt", 100)
			cmd.Stop()
			elapsed := time.Since(startTime)

			// Verify Stop() returns at timeout or command completion
			// Add 100ms buffer time
			expectedMaxDuration := min(tt.sleepDuration, tt.timeout) + 100*time.Millisecond
			if elapsed > expectedMaxDuration {
				t.Errorf("Stop() took too long: %v, expected max %v", elapsed, expectedMaxDuration)
			}

			data, err := os.ReadFile(outputFile)
			hasOutput := err == nil && strings.Contains(string(data), marker)

			if tt.shouldComplete && !hasOutput {
				t.Errorf("Expected command to complete within timeout, but it did not")
			}
			if !tt.shouldComplete && hasOutput {
				t.Errorf("Expected command to be killed by timeout, but it completed")
			}
		})
	}
}

func TestCommand_EmptyCommand(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "p2pcp-cmd-empty-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputFile := filepath.Join(tmpDir, "empty.txt")

	// Test empty commands vs actual commands to verify empty commands are not executed
	t.Run("empty onComplete does not execute", func(t *testing.T) {
		os.Remove(outputFile)

		// Only onCompleteCommand is empty string
		cmd := NewCommand("", "echo 'file' >> "+outputFile, 5*time.Second)
		cmd.Run()

		cmd.RunOnComplete(10, 1000, "5s")
		cmd.Stop()

		// Empty onComplete should not execute, so no file should be created
		if _, err := os.Stat(outputFile); err == nil {
			t.Error("Empty onComplete command should not create any output")
		}
	})

	t.Run("empty onEachFileComplete does not execute", func(t *testing.T) {
		os.Remove(outputFile)

		// Only onEachFileCompleteCommand is empty string
		cmd := NewCommand("echo 'complete' >> "+outputFile, "", 5*time.Second)
		cmd.Run()

		cmd.RunOnEachFileComplete("/test/file.txt", 100)
		cmd.Stop()

		// Empty onEachFileComplete should not execute, so no file should be created
		if _, err := os.Stat(outputFile); err == nil {
			t.Error("Empty onEachFileComplete command should not create any output")
		}
	})

	t.Run("both empty commands do not execute", func(t *testing.T) {
		os.Remove(outputFile)

		// Both commands are empty strings
		cmd := NewCommand("", "", 5*time.Second)
		cmd.Run()

		cmd.RunOnComplete(10, 1000, "5s")
		cmd.RunOnEachFileComplete("/test/file.txt", 100)
		cmd.Stop()

		// Nothing should execute, so no file should be created
		if _, err := os.Stat(outputFile); err == nil {
			t.Error("Both empty commands should not create any output")
		}
	})
}

func TestCommand_MixedCommands(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "p2pcp-cmd-mixed-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputFile := filepath.Join(tmpDir, "mixed.txt")

	onCompleteCmd := "echo 'COMPLETE: files=" + TemplateFileCount + "' >> " + outputFile
	onFileCmd := "echo 'FILE: " + TemplateFilePath + "' >> " + outputFile

	cmd := NewCommand(onCompleteCmd, onFileCmd, 5*time.Second)

	cmd.Run()

	cmd.RunOnEachFileComplete("/data/file1.txt", 100)
	cmd.RunOnEachFileComplete("/data/file2.txt", 200)
	cmd.RunOnComplete(2, 300, "1s")
	cmd.RunOnEachFileComplete("/data/file3.txt", 150)

	cmd.Stop()

	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	output := string(data)

	if !strings.Contains(output, "FILE: /data/file1.txt") {
		t.Error("Expected file1 in output")
	}
	if !strings.Contains(output, "FILE: /data/file2.txt") {
		t.Error("Expected file2 in output")
	}
	if !strings.Contains(output, "FILE: /data/file3.txt") {
		t.Error("Expected file3 in output")
	}
	if !strings.Contains(output, "COMPLETE: files=2") {
		t.Error("Expected complete command in output")
	}
}
