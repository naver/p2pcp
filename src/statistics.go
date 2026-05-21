// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/naver/p2pcp/utils"
	"github.com/naver/p2pcp/version"
)

type Statistics struct {
	IsUse bool
	Lock  sync.RWMutex
	Data  map[string]string

	IsSetBytesTransmittedTotal atomic.Bool
	IsSetBytesReceivedTotal    atomic.Bool

	BytesTransmittedTotal   atomic.Int64
	BytesReceivedTotal      atomic.Int64
	ChunksReceivedTotal     atomic.Int64
	CumulativeDurationTotal utils.AtomicDuration
	Elapsed                 time.Duration
}

func NewStatistics(isUse bool) *Statistics {
	return &Statistics{
		IsUse: isUse,
		Data:  map[string]string{},
	}
}

func (stat *Statistics) GetBytesReceivedTotal() int64 {
	return stat.BytesReceivedTotal.Load()
}

func (stat *Statistics) AddBytesTransmittedTotal(n int64) {
	stat.IsSetBytesTransmittedTotal.Store(true)

	stat.BytesTransmittedTotal.Add(n)
}

func (stat *Statistics) AddDownloader(name string, bytesReceivedTotal int64, chunksReceivedTotal int64, cumulativeDurationTotal time.Duration) {
	stat.IsSetBytesReceivedTotal.Store(true)

	stat.BytesReceivedTotal.Add(bytesReceivedTotal)
	stat.ChunksReceivedTotal.Add(chunksReceivedTotal)
	stat.CumulativeDurationTotal.Add(cumulativeDurationTotal)

	if stat.IsUse {
		stat.Lock.Lock()
		defer stat.Lock.Unlock()

		stat.Data[fmt.Sprintf("[%s] received_bytes", name)] = fmt.Sprint(bytesReceivedTotal)
		stat.Data[fmt.Sprintf("[%s] received_chunks", name)] = fmt.Sprint(chunksReceivedTotal)
		stat.Data[fmt.Sprintf("[%s] cumulative_transfer_time", name)] = cumulativeDurationTotal.String()
	}
}

func (stat *Statistics) SetElapsed(elapsed time.Duration) {
	stat.Elapsed = elapsed
}

func (stat *Statistics) PrintAll() {
	if stat.IsUse {
		Print := func(data map[string]string) {
			var keys []string
			for key := range data {
				keys = append(keys, key)
			}
			sort.Strings(keys)

			for _, key := range keys {
				utils.Printf("%s = %s", key, data[key])
			}
		}

		utils.Printf("%s download statistics information:", version.GetVersion())

		stat.Lock.RLock()
		Print(stat.Data)
		stat.Lock.RUnlock()

		TotalData := map[string]string{}

		if stat.IsSetBytesReceivedTotal.Load() {
			TotalData["[Total] received_bytes"] = fmt.Sprint(stat.BytesReceivedTotal.Load())
			TotalData["[Total] received_chunks"] = fmt.Sprint(stat.ChunksReceivedTotal.Load())
			TotalData["[Total] cumulative_transfer_time"] = stat.CumulativeDurationTotal.Load().String()
			TotalData["[Total] elapsed_time"] = stat.Elapsed.String()

			elapsedSeconds := int64(stat.Elapsed.Seconds())
			if elapsedSeconds == 0 {
				elapsedSeconds = 1
			}
			TotalData["[Total] transfer_bytes_per_seconds"] = fmt.Sprint(stat.BytesReceivedTotal.Load() / elapsedSeconds)
		}
		if stat.IsSetBytesTransmittedTotal.Load() {
			TotalData["[Total] transmitted_bytes"] = fmt.Sprint(stat.BytesTransmittedTotal.Load())
		}

		Print(TotalData)
	}
}
