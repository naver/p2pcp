// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package utils

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"
)

// testLoggerOutput is a helper struct for capturing logger output
type testLoggerOutput struct {
	stdout bytes.Buffer
	stderr bytes.Buffer
}

// withTestLogger replaces the singleton logger for testing and
// captures stdout/stderr output.
// Restores original state after test completes.
func withTestLogger(t *testing.T, fn func()) *testLoggerOutput {
	t.Helper()

	// Backup original singleton
	originalInstance := loggerInstance
	originalOnce := loggerOnce
	defer func() {
		loggerInstance = originalInstance
		loggerOnce = originalOnce
	}()

	// Reset sync.Once to allow new logger creation
	loggerOnce = &sync.Once{}
	loggerInstance = nil

	// Create pipes
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stderr pipe: %v", err)
	}

	// Replace os.Stdout/Stderr
	origStdout, origStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdoutW, stderrW

	// Create new logger (GetLogger passes sync.Once and binds to pipes)
	_ = GetLogger()

	// Restore os.Stdout/Stderr (logger already holds pipe fd)
	os.Stdout, os.Stderr = origStdout, origStderr

	// Run test
	fn()

	// Flush and close pipes
	loggerInstance.Close()
	stdoutW.Close()
	stderrW.Close()

	// Read output
	output := &testLoggerOutput{}
	io.Copy(&output.stdout, stdoutR)
	io.Copy(&output.stderr, stderrR)
	stdoutR.Close()
	stderrR.Close()

	return output
}

func TestNewLogger(t *testing.T) {
	logger := NewLogger()
	if logger == nil {
		t.Fatal("Expected non-nil logger")
	}

	if logger.stdoutLogger == nil {
		t.Error("Expected non-nil stdout logger")
	}

	if logger.stderrorLogger == nil {
		t.Error("Expected non-nil stderr logger")
	}

	if logger.stacktraceLevel == nil {
		t.Error("Expected non-nil stacktrace level")
	}

	if logger.logLevel == nil {
		t.Error("Expected non-nil log level")
	}

	// Verify initial log level is Info
	if logger.logLevel.Level() != zap.InfoLevel {
		t.Errorf("Expected initial log level to be InfoLevel, got %v", logger.logLevel.Level())
	}

	// Verify initial stacktrace level is DPanic
	if logger.stacktraceLevel.Level() != zap.DPanicLevel {
		t.Errorf("Expected stacktrace level to be DPanicLevel, got %v", logger.stacktraceLevel.Level())
	}
}

func TestLogger_SetVerbose(t *testing.T) {
	logger := NewLogger()

	// Verify initial level is Info
	if logger.logLevel.Level() != zap.InfoLevel {
		t.Errorf("Expected initial level to be InfoLevel, got %v", logger.logLevel.Level())
	}

	// Test enabling verbose mode
	logger.SetVerbose(true)
	if logger.logLevel.Level() != zap.DebugLevel {
		t.Errorf("Expected level to be DebugLevel after SetVerbose(true), got %v", logger.logLevel.Level())
	}

	// Test disabling verbose mode
	logger.SetVerbose(false)
	if logger.logLevel.Level() != zap.InfoLevel {
		t.Errorf("Expected level to be InfoLevel after SetVerbose(false), got %v", logger.logLevel.Level())
	}

	// Test toggling multiple times
	logger.SetVerbose(true)
	logger.SetVerbose(true) // idempotent
	if logger.logLevel.Level() != zap.DebugLevel {
		t.Errorf("Expected level to remain DebugLevel, got %v", logger.logLevel.Level())
	}

	logger.SetVerbose(false)
	logger.SetVerbose(false) // idempotent
	if logger.logLevel.Level() != zap.InfoLevel {
		t.Errorf("Expected level to remain InfoLevel, got %v", logger.logLevel.Level())
	}
}

func TestLogger_Close(t *testing.T) {
	logger := NewLogger()

	// Close should not panic and should be idempotent
	logger.Close()
	logger.Close() // second close should also not panic
}

func TestGetLogger(t *testing.T) {
	// GetLogger should return same instance (singleton)
	logger1 := GetLogger()
	logger2 := GetLogger()

	if logger1 != logger2 {
		t.Error("Expected GetLogger to return same instance")
	}

	if logger1 == nil {
		t.Error("Expected non-nil logger from GetLogger")
	}
}

func TestDebugPrintf(t *testing.T) {
	t.Run("verbose enabled", func(t *testing.T) {
		output := withTestLogger(t, func() {
			GetLogger().SetVerbose(true)
			DebugPrintf("debug message: %s", "hello")
		})

		stdout := output.stdout.String()
		if !strings.Contains(stdout, "DEBUG") {
			t.Errorf("expected DEBUG level in output, got: %s", stdout)
		}
		if !strings.Contains(stdout, "debug message: hello") {
			t.Errorf("expected message content in output, got: %s", stdout)
		}
	})

	t.Run("verbose disabled", func(t *testing.T) {
		output := withTestLogger(t, func() {
			GetLogger().SetVerbose(false)
			DebugPrintf("this should be filtered: %d", 123)
		})

		stdout := output.stdout.String()
		if strings.Contains(stdout, "this should be filtered") {
			t.Errorf("debug message should be filtered when verbose is disabled, got: %s", stdout)
		}
	})
}

func TestErrorPrintf(t *testing.T) {
	t.Run("verbose disabled", func(t *testing.T) {
		output := withTestLogger(t, func() {
			GetLogger().SetVerbose(false)
			ErrorPrintf("error message: %s", "failure")
		})

		stderr := output.stderr.String()
		if !strings.Contains(stderr, "ERROR") {
			t.Errorf("expected ERROR level in output, got: %s", stderr)
		}
		if !strings.Contains(stderr, "error message: failure") {
			t.Errorf("expected message content in output, got: %s", stderr)
		}
	})

	t.Run("verbose enabled", func(t *testing.T) {
		output := withTestLogger(t, func() {
			GetLogger().SetVerbose(true)
			ErrorPrintf("error in verbose: %d", 500)
		})

		stderr := output.stderr.String()
		if !strings.Contains(stderr, "ERROR") {
			t.Errorf("expected ERROR level in output, got: %s", stderr)
		}
		if !strings.Contains(stderr, "error in verbose: 500") {
			t.Errorf("expected message content in output, got: %s", stderr)
		}
	})
}

func TestPrintf(t *testing.T) {
	t.Run("verbose disabled", func(t *testing.T) {
		output := withTestLogger(t, func() {
			GetLogger().SetVerbose(false)
			Printf("info message: %s", "data")
		})

		stdout := output.stdout.String()
		if !strings.Contains(stdout, "INFO") {
			t.Errorf("expected INFO level in output, got: %s", stdout)
		}
		if !strings.Contains(stdout, "info message: data") {
			t.Errorf("expected message content in output, got: %s", stdout)
		}
	})

	t.Run("verbose enabled", func(t *testing.T) {
		output := withTestLogger(t, func() {
			GetLogger().SetVerbose(true)
			Printf("info in verbose: %d", 42)
		})

		stdout := output.stdout.String()
		if !strings.Contains(stdout, "INFO") {
			t.Errorf("expected INFO level in output, got: %s", stdout)
		}
		if !strings.Contains(stdout, "info in verbose: 42") {
			t.Errorf("expected message content in output, got: %s", stdout)
		}
	})
}

func TestLogger_Integration(t *testing.T) {
	output := withTestLogger(t, func() {
		logger := GetLogger()

		// Log all levels in verbose mode
		logger.SetVerbose(true)
		DebugPrintf("debug: %d", 123)
		Printf("info: %s", "test")
		ErrorPrintf("error: %v", "failure")

		// Debug is filtered after verbose off
		logger.SetVerbose(false)
		DebugPrintf("debug filtered")
		Printf("info after verbose off")
	})

	stdout := output.stdout.String()
	stderr := output.stderr.String()

	// Verify debug output when verbose is on
	if !strings.Contains(stdout, "debug: 123") {
		t.Errorf("expected debug message when verbose=true, got stdout: %s", stdout)
	}

	// Verify info output
	if !strings.Contains(stdout, "info: test") {
		t.Errorf("expected info message, got stdout: %s", stdout)
	}

	// Error outputs to stderr
	if !strings.Contains(stderr, "error: failure") {
		t.Errorf("expected error message in stderr, got: %s", stderr)
	}

	// Debug is filtered after verbose off
	if strings.Contains(stdout, "debug filtered") {
		t.Errorf("debug should be filtered when verbose=false, got stdout: %s", stdout)
	}

	// Info still outputs after verbose off
	if !strings.Contains(stdout, "info after verbose off") {
		t.Errorf("info should appear when verbose=false, got stdout: %s", stdout)
	}
}
