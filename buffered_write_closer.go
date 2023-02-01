// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"bufio"
	"io"
	"sync"
	"time"
)

type BufferedWriteCloser struct {
	mu     sync.Mutex
	writer *bufio.Writer
	ticker *time.Ticker
	stop   chan struct{} // closed when flushLoop should stop
	done   chan struct{} // closed when flushLoop has stopped
}

func NewBufferedWriteCloser(writer io.Writer, size int, flushInterval time.Duration) io.WriteCloser {
	writeCloser := &BufferedWriteCloser{
		writer: bufio.NewWriterSize(writer, size),
		ticker: time.NewTicker(flushInterval),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}

	go writeCloser.flushLoop()

	return writeCloser
}

func (l *BufferedWriteCloser) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.ticker.Stop()
	close(l.stop) // tell flushLoop to stop
	<-l.done      // and wait until it has

	return nil
}

func (l *BufferedWriteCloser) Write(bs []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.writer.Write(bs)
}

func (l *BufferedWriteCloser) flush() error {
	return l.writer.Flush()
}

func (l *BufferedWriteCloser) flushLoop() {
	defer close(l.done)

	for {
		select {
		case <-l.ticker.C:
			l.mu.Lock()
			l.flush()
			l.mu.Unlock()
		case <-l.stop:
			l.flush()
			return
		}
	}
}
