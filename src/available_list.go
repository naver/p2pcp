// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"time"
)

type AvailableList struct {
	Timestamp              time.Time `json:"timestamp"`
	IsCompleted            bool      `json:"isCompleted"`
	TransferChunkIndexList []int     `json:"transferChunkIndexList"`
}

func NewAvailableList() AvailableList {
	return AvailableList{
		Timestamp: time.Now(),
	}
}
