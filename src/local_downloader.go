// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"io"

	"github.com/naver/p2pcp/storage"
	"github.com/naver/p2pcp/transfer"
)

type LocalDownloader struct {
	chunkManager *transfer.ChunkManager
	queue        *transfer.ChunkQueue
	dirSrc       storage.Storage
}

func NewLocalDownloader(chunkManager *transfer.ChunkManager, dirSrc storage.Storage) *LocalDownloader {
	downloader := &LocalDownloader{
		chunkManager: chunkManager,
		queue:        transfer.NewChunkQueue(),
		dirSrc:       dirSrc,
	}

	indices := make([]int, chunkManager.GetChunkCount())
	for i := range chunkManager.GetChunkCount() {
		indices[i] = i
	}
	downloader.queue.PushShuffledList(indices)

	return downloader
}

func (downloader *LocalDownloader) Name() string {
	return "Local"
}

func (downloader *LocalDownloader) GetQueue() *transfer.ChunkQueue {
	return downloader.queue
}

func (downloader *LocalDownloader) RequestDownload(chunkIndex int, withChecksum bool) (uint64, io.ReadCloser, error) {
	if withChecksum {
		checksum, err := downloader.chunkManager.ComputeChunkChecksum(downloader.dirSrc, chunkIndex)
		if err != nil {
			return 0, nil, err
		}

		readCloser := downloader.chunkManager.GetChunkReader(downloader.dirSrc, chunkIndex)
		return checksum, readCloser, nil
	} else {
		readCloser := downloader.chunkManager.GetChunkReader(downloader.dirSrc, chunkIndex)
		return 0, readCloser, nil
	}
}

func (downloader *LocalDownloader) IsValid() bool {
	return true
}

func (downloader *LocalDownloader) Fin() {
}
