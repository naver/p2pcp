// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"sync/atomic"
	"time"
)

type AtomicDuration struct {
	v time.Duration
}

func (x *AtomicDuration) Load() time.Duration {
	return time.Duration(atomic.LoadInt64((*int64)(&x.v)))
}

func (x *AtomicDuration) Store(val time.Duration) {
	atomic.StoreInt64((*int64)(&x.v), int64(val))
}

func (x *AtomicDuration) Swap(new time.Duration) time.Duration {
	return time.Duration(atomic.SwapInt64((*int64)(&x.v), int64(new)))
}

func (x *AtomicDuration) CompareAndSwap(old, new time.Duration) bool {
	return atomic.CompareAndSwapInt64((*int64)(&x.v), int64(old), int64(new))
}

func (x *AtomicDuration) Add(delta time.Duration) time.Duration {
	return time.Duration(atomic.AddInt64((*int64)(&x.v), int64(delta)))
}

type AtomicTime struct {
	v atomic.Value
}

func (x *AtomicTime) Load() time.Time {
	val := x.v.Load()
	if val == nil {
		return time.Time{}
	} else {
		return val.(time.Time)
	}
}

func (x *AtomicTime) Store(val time.Time) {
	x.v.Store(val)
}

func (x *AtomicTime) Swap(new time.Time) time.Time {
	val := x.v.Swap(new)
	if val == nil {
		return time.Time{}
	} else {
		return val.(time.Time)
	}
}

func (x *AtomicTime) CompareAndSwap(old, new time.Time) bool {
	return x.v.CompareAndSwap(old, new)
}
