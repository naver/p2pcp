// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package types

import (
	"sort"
	"sync/atomic"
	"time"

	"github.com/naver/p2pcp/utils"
)

type FileRange struct {
	FileIndex   int   `json:"index"`
	StartOffset int64 `json:"from"`
	EndOffset   int64 `json:"to"`
}

type ChunkStatus int32

const (
	ChunkStatusPending ChunkStatus = iota
	ChunkStatusDownloading
	ChunkStatusDone
)

type Chunk struct {
	FileRanges []FileRange      `json:"fileChunks"`
	status     atomic.Int32     `json:"-"`
	updatedAt  utils.AtomicTime `json:"-"`
}

func BuildChunks(files []FileEntry, chunkSize int64, maxFileCountPerChunk int) []Chunk {
	chunkList := make([]Chunk, 0)
	if len(files) == 0 {
		return chunkList
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})

	chunk := Chunk{}
	var currentChunkSize int64

	newChunk := func() {
		chunkList = append(chunkList, chunk.copy())
		chunk = Chunk{}
		currentChunkSize = 0
	}
	addToChunk := func(index int, startOffset int64, endOffset int64) {
		chunk.FileRanges = append(chunk.FileRanges, FileRange{index, startOffset, endOffset})
		currentChunkSize += (endOffset - startOffset)
	}

	for index := range files {
		file := &files[index]

		var startOffset int64
		remainFilesize := file.Size
		for remainFilesize > 0 {
			file.IncrementRemainingChunkCount()

			if currentChunkSize+remainFilesize < chunkSize {
				// Can fit remaining file in TransferChunk
				addToChunk(index, startOffset, file.Size)

				if len(chunk.FileRanges) == maxFileCountPerChunk {
					// transferChunk.FileChunks length reached flagMaxFileCountPerChunk, split TransferChunk
					newChunk()
				}

				// remainFilesize = 0
				break
			} else {
				// Cannot fit remaining file in TransferChunk, split it
				size := chunkSize - currentChunkSize
				addToChunk(index, startOffset, startOffset+size)
				newChunk()

				startOffset += size
				remainFilesize -= size
			}
		}
	}

	if len(chunk.FileRanges) > 0 {
		chunkList = append(chunkList, chunk.copy())
	}

	return chunkList
}

func (c *Chunk) copy() Chunk {
	return Chunk{FileRanges: c.FileRanges}
}

func (c *Chunk) Size() int64 {
	var length int64
	for _, fileChunk := range c.FileRanges {
		length += (fileChunk.EndOffset - fileChunk.StartOffset)
	}
	return length
}

func (c *Chunk) GetStatus() ChunkStatus {
	return ChunkStatus(c.status.Load())
}

func (c *Chunk) SetStatus(status ChunkStatus) {
	c.status.Store(int32(status))
}

func (c *Chunk) UpdateStatus(oldStatus, newStatus ChunkStatus) bool {
	return c.status.CompareAndSwap(int32(oldStatus), int32(newStatus))
}

func (c *Chunk) GetUpdatedAt() time.Time {
	return c.updatedAt.Load()
}

func (c *Chunk) SetUpdatedAt(updatedAt time.Time) {
	c.updatedAt.Store(updatedAt)
}
