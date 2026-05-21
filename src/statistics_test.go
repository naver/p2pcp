// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"testing"
	"time"
)

func TestNewStatistics(t *testing.T) {
	tests := []struct {
		name  string
		isUse bool
	}{
		{
			name:  "statistics enabled",
			isUse: true,
		},
		{
			name:  "statistics disabled",
			isUse: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stat := NewStatistics(tt.isUse)

			if stat.IsUse != tt.isUse {
				t.Errorf("IsUse = %v, want %v", stat.IsUse, tt.isUse)
			}

			if stat.Data == nil {
				t.Error("Data should not be nil")
			}
		})
	}
}

func TestStatistics_GetBytesReceivedTotal(t *testing.T) {
	stat := NewStatistics(false)

	// Verify initial value
	if got := stat.GetBytesReceivedTotal(); got != 0 {
		t.Errorf("GetBytesReceivedTotal() = %v, want 0", got)
	}

	// Verify after setting value
	stat.BytesReceivedTotal.Store(1000)
	if got := stat.GetBytesReceivedTotal(); got != 1000 {
		t.Errorf("GetBytesReceivedTotal() = %v, want 1000", got)
	}
}

func TestStatistics_AddBytesTransmittedTotal(t *testing.T) {
	stat := NewStatistics(false)

	stat.AddBytesTransmittedTotal(100)
	if got := stat.BytesTransmittedTotal.Load(); got != 100 {
		t.Errorf("BytesTransmittedTotal = %v, want 100", got)
	}

	stat.AddBytesTransmittedTotal(50)
	if got := stat.BytesTransmittedTotal.Load(); got != 150 {
		t.Errorf("BytesTransmittedTotal = %v, want 150", got)
	}

	if !stat.IsSetBytesTransmittedTotal.Load() {
		t.Error("IsSetBytesTransmittedTotal should be true")
	}
}

func TestStatistics_AddDownloader(t *testing.T) {
	t.Run("with statistics enabled", func(t *testing.T) {
		stat := NewStatistics(true)

		stat.AddDownloader("TestDownloader", 1000, 10, 5*time.Second)

		if got := stat.BytesReceivedTotal.Load(); got != 1000 {
			t.Errorf("BytesReceivedTotal = %v, want 1000", got)
		}

		if got := stat.ChunksReceivedTotal.Load(); got != 10 {
			t.Errorf("ChunksReceivedTotal = %v, want 10", got)
		}

		if got := stat.CumulativeDurationTotal.Load(); got != 5*time.Second {
			t.Errorf("CumulativeDurationTotal = %v, want 5s", got)
		}

		if !stat.IsSetBytesReceivedTotal.Load() {
			t.Error("IsSetBytesReceivedTotal should be true")
		}

		// Verify Data map
		stat.Lock.RLock()
		defer stat.Lock.RUnlock()

		if stat.Data["[TestDownloader] received_bytes"] != "1000" {
			t.Errorf("Data[received_bytes] = %v, want 1000", stat.Data["[TestDownloader] received_bytes"])
		}

		if stat.Data["[TestDownloader] received_chunks"] != "10" {
			t.Errorf("Data[received_chunks] = %v, want 10", stat.Data["[TestDownloader] received_chunks"])
		}

		if stat.Data["[TestDownloader] cumulative_transfer_time"] != "5s" {
			t.Errorf("Data[cumulative_transfer_time] = %v, want 5s", stat.Data["[TestDownloader] cumulative_transfer_time"])
		}
	})

	t.Run("with statistics disabled", func(t *testing.T) {
		stat := NewStatistics(false)

		stat.AddDownloader("TestDownloader", 1000, 10, 5*time.Second)

		// Statistics are still updated
		if got := stat.BytesReceivedTotal.Load(); got != 1000 {
			t.Errorf("BytesReceivedTotal = %v, want 1000", got)
		}

		// Data map should be empty
		stat.Lock.RLock()
		defer stat.Lock.RUnlock()

		if len(stat.Data) != 0 {
			t.Errorf("Data should be empty, got %v", stat.Data)
		}
	})

	t.Run("multiple downloaders", func(t *testing.T) {
		stat := NewStatistics(true)

		stat.AddDownloader("Local", 500, 5, 2*time.Second)
		stat.AddDownloader("Peer:192.168.1.1:8080", 1500, 15, 8*time.Second)

		if got := stat.BytesReceivedTotal.Load(); got != 2000 {
			t.Errorf("BytesReceivedTotal = %v, want 2000", got)
		}

		if got := stat.ChunksReceivedTotal.Load(); got != 20 {
			t.Errorf("ChunksReceivedTotal = %v, want 20", got)
		}

		if got := stat.CumulativeDurationTotal.Load(); got != 10*time.Second {
			t.Errorf("CumulativeDurationTotal = %v, want 10s", got)
		}
	})
}

func TestStatistics_SetElapsed(t *testing.T) {
	stat := NewStatistics(false)

	elapsed := 10 * time.Second
	stat.SetElapsed(elapsed)

	if stat.Elapsed != elapsed {
		t.Errorf("Elapsed = %v, want %v", stat.Elapsed, elapsed)
	}
}

func TestStatistics_PrintAll(t *testing.T) {
	t.Run("with statistics disabled", func(t *testing.T) {
		stat := NewStatistics(false)
		stat.AddDownloader("Test", 1000, 10, 5*time.Second)

		// PrintAll does nothing when IsUse is false - verify no panic occurs
		stat.PrintAll()
	})

	t.Run("with statistics enabled", func(t *testing.T) {
		stat := NewStatistics(true)
		stat.AddDownloader("Local", 1000, 10, 5*time.Second)
		stat.AddBytesTransmittedTotal(500)
		stat.SetElapsed(10 * time.Second)

		// Verify PrintAll executes without panic
		stat.PrintAll()
	})

	t.Run("with zero elapsed time", func(t *testing.T) {
		stat := NewStatistics(true)
		stat.AddDownloader("Test", 1000, 10, 5*time.Second)
		stat.SetElapsed(0)

		// When elapsedSeconds is 0, it's set to 1, so no divide by zero occurs
		stat.PrintAll()
	})

	t.Run("with only transmitted bytes", func(t *testing.T) {
		stat := NewStatistics(true)
		stat.AddBytesTransmittedTotal(1000)

		// Works correctly with only transmitted bytes
		stat.PrintAll()
	})
}

func TestStatistics_Concurrent(t *testing.T) {
	stat := NewStatistics(true)
	done := make(chan struct{})

	// Update statistics concurrently from multiple goroutines
	for i := 0; i < 10; i++ {
		go func(idx int) {
			for j := 0; j < 100; j++ {
				stat.AddBytesTransmittedTotal(1)
				stat.AddDownloader("Test", 1, 1, time.Millisecond)
			}
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if got := stat.BytesTransmittedTotal.Load(); got != 1000 {
		t.Errorf("BytesTransmittedTotal = %v, want 1000", got)
	}

	if got := stat.BytesReceivedTotal.Load(); got != 1000 {
		t.Errorf("BytesReceivedTotal = %v, want 1000", got)
	}
}
