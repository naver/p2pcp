// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestAtomicDuration_Load(t *testing.T) {
	tests := []struct {
		name string
		want time.Duration
	}{
		{
			"load",
			3 * time.Nanosecond,
		},
		{
			"load",
			5 * time.Microsecond,
		},
		{
			"load",
			100 * time.Millisecond,
		},
		{
			"load",
			5000 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &AtomicDuration{
				v: tt.want,
			}
			if got := x.Load(); got != tt.want {
				t.Errorf("AtomicDuration.Load() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAtomicDuration_Store(t *testing.T) {
	tests := []struct {
		name string
		want time.Duration
	}{
		{
			"store",
			3 * time.Nanosecond,
		},
		{
			"store",
			5 * time.Microsecond,
		},
		{
			"store",
			100 * time.Millisecond,
		},
		{
			"store",
			5000 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &AtomicDuration{
				v: time.Duration(123),
			}
			x.Store(tt.want)
			if got := x.Load(); got != tt.want {
				t.Errorf("AtomicDuration.Store() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAtomicDuration_Swap(t *testing.T) {
	tests := []struct {
		name string
		want time.Duration
	}{
		{
			"swap",
			3 * time.Nanosecond,
		},
		{
			"swap",
			5 * time.Microsecond,
		},
		{
			"swap",
			100 * time.Millisecond,
		},
		{
			"swap",
			5000 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &AtomicDuration{
				v: tt.want,
			}
			new := time.Duration(123)
			if got := x.Swap(new); got != tt.want {
				t.Errorf("AtomicDuration.Swap() = %v, want %v", got, tt.want)
			}
			if got := x.Load(); got != new {
				t.Errorf("AtomicDuration.Load() = %v, want %v", got, new)
			}
		})
	}
}

func TestAtomicDuration_CompareAndSwap(t *testing.T) {
	type fields struct {
		Duration time.Duration
	}
	type args struct {
		old time.Duration
		new time.Duration
	}
	tests := []struct {
		name        string
		fields      fields
		args        args
		wantSwapped bool
	}{
		{
			"cas",
			fields{3 * time.Second},
			args{3 * time.Second, 5 * time.Second},
			true,
		},
		{
			"cas",
			fields{3 * time.Second},
			args{1 * time.Second, 5 * time.Second},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &AtomicDuration{
				v: tt.fields.Duration,
			}
			if gotSwapped := x.CompareAndSwap(tt.args.old, tt.args.new); gotSwapped != tt.wantSwapped {
				t.Errorf("AtomicDuration.CompareAndSwap() = %v, want %v", gotSwapped, tt.wantSwapped)
			}
		})
	}
}

func TestAtomicDuration_Add(t *testing.T) {
	type fields struct {
		Duration time.Duration
	}
	type args struct {
		delta time.Duration
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantNew time.Duration
	}{
		{
			"add",
			fields{3 * time.Nanosecond},
			args{2 * time.Nanosecond},
			5 * time.Nanosecond,
		},
		{
			"add",
			fields{3 * time.Second},
			args{2 * time.Second},
			5 * time.Second,
		},
		{
			"add",
			fields{3 * time.Second},
			args{-1 * time.Second},
			2 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &AtomicDuration{
				v: tt.fields.Duration,
			}
			if gotNew := x.Add(tt.args.delta); gotNew != tt.wantNew {
				t.Errorf("AtomicDuration.Add() = %v, want %v", gotNew, tt.wantNew)
			}
		})
	}
}

func TestAtomicDuration_Race(t *testing.T) {
	//go test -race -v custom_atomic_test.go custom_atomic.go

	var wg sync.WaitGroup

	var testDuration AtomicDuration

	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10000; j++ {
				load := testDuration.Load()
				load += 10 * time.Second
				testDuration.Store(load)

				old := testDuration.Swap(2 * time.Second)
				testDuration.CompareAndSwap(old, 3*time.Second)
				testDuration.Add(5 * time.Second)
			}
		}()
	}
	wg.Wait()
}

func TestAtomicTime_StoreLoad(t *testing.T) {
	tests := []struct {
		name string
		want time.Time
	}{
		{
			"sotre&load",
			time.Time{},
		},
		{
			"sotre&load",
			time.Unix(12345, 12345),
		},
		{
			"sotre&load",
			time.Now(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &AtomicTime{}
			empty := time.Time{}
			if tt.want != empty {
				x.Store(tt.want)
			}

			if got := x.Load(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AtomicTime.Load() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAtomicTime_Swap(t *testing.T) {
	tests := []struct {
		name string
		want time.Time
	}{
		{
			"swap",
			time.Time{},
		},
		{
			"swap",
			time.Unix(12345, 12345),
		},
		{
			"swap",
			time.Now(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &AtomicTime{}
			empty := time.Time{}
			if tt.want != empty {
				x.Store(tt.want)
			}
			new := time.Unix(67890, 67890)
			if got := x.Swap(new); got != tt.want {
				t.Errorf("AtomicTime.Swap() = %v, want %v", got, tt.want)
			}
			if got := x.Load(); got != new {
				t.Errorf("AtomicTime.Load() = %v, want %v", got, new)
			}
		})
	}
}

func TestAtomicTime_CompareAndSwap(t *testing.T) {
	type fields struct {
		v time.Time
	}
	type args struct {
		old time.Time
		new time.Time
	}
	tests := []struct {
		name        string
		fields      fields
		args        args
		wantSwapped bool
	}{
		{
			"cas",
			fields{time.Unix(12345, 12345)},
			args{time.Unix(12345, 12345), time.Unix(67890, 67890)},
			true,
		},
		{
			"cas",
			fields{time.Unix(12345, 12345)},
			args{time.Unix(67890, 67890), time.Now()},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &AtomicTime{}
			x.Store(tt.fields.v)
			if gotSwapped := x.CompareAndSwap(tt.args.old, tt.args.new); gotSwapped != tt.wantSwapped {
				t.Errorf("AtomicTime.CompareAndSwap() = %v, want %v", gotSwapped, tt.wantSwapped)
			}
		})
	}
}

func TestAtomicTime_Race(t *testing.T) {
	//go test -race -v custom_atomic_test.go custom_atomic.go

	var wg sync.WaitGroup

	var testTime AtomicTime

	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10000; j++ {
				load := testTime.Load()
				testTime.Store(load.Add(10 * time.Second))

				old := testTime.Swap(time.Unix(12345, 12345))
				testTime.CompareAndSwap(old, time.Unix(67890, 67890))
			}
		}()
	}
	wg.Wait()
}
