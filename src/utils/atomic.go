// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package utils

import (
	"sync/atomic"
	"time"
)

type AtomicDuration struct {
	value time.Duration
}

func (atomicDuration *AtomicDuration) Load() time.Duration {
	return time.Duration(atomic.LoadInt64((*int64)(&atomicDuration.value)))
}

func (atomicDuration *AtomicDuration) Store(val time.Duration) {
	atomic.StoreInt64((*int64)(&atomicDuration.value), int64(val))
}

func (atomicDuration *AtomicDuration) Swap(new time.Duration) time.Duration {
	return time.Duration(atomic.SwapInt64((*int64)(&atomicDuration.value), int64(new)))
}

func (atomicDuration *AtomicDuration) CompareAndSwap(old, new time.Duration) bool {
	return atomic.CompareAndSwapInt64((*int64)(&atomicDuration.value), int64(old), int64(new))
}

func (atomicDuration *AtomicDuration) Add(delta time.Duration) time.Duration {
	return time.Duration(atomic.AddInt64((*int64)(&atomicDuration.value), int64(delta)))
}

type AtomicTime struct {
	value atomic.Value
}

func (atomicTime *AtomicTime) Load() time.Time {
	val := atomicTime.value.Load()
	if val == nil {
		return time.Time{}
	} else {
		return val.(time.Time)
	}
}

func (atomicTime *AtomicTime) Store(val time.Time) {
	atomicTime.value.Store(val)
}

func (atomicTime *AtomicTime) Swap(new time.Time) time.Time {
	val := atomicTime.value.Swap(new)
	if val == nil {
		return time.Time{}
	} else {
		return val.(time.Time)
	}
}

func (atomicTime *AtomicTime) CompareAndSwap(old, new time.Time) bool {
	return atomicTime.value.CompareAndSwap(old, new)
}
