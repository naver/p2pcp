// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"testing"
	"time"
)

func TestNewAvailableList(t *testing.T) {
	before := time.Now()
	al := NewAvailableList()
	after := time.Now()

	if al.Timestamp.Before(before) || al.Timestamp.After(after) {
		t.Errorf("timestamp out of expected range: %v", al.Timestamp)
	}

	if al.IsCompleted {
		t.Error("IsCompleted should be false by default")
	}

	if al.TransferChunkIndexList != nil {
		t.Error("TransferChunkIndexList should be nil")
	}
}
