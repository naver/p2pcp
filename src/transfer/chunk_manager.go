// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package transfer

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/naver/p2pcp/peers"
	"github.com/naver/p2pcp/storage"
	"github.com/naver/p2pcp/types"
	"github.com/zeebo/xxh3"
	"golang.org/x/sync/singleflight"
)

type ChunkManager struct {
	manifest           *Manifest
	manifestJson       []byte
	chunks             []types.Chunk
	chunkChecksums     []uint64
	chunkChecksumGroup singleflight.Group
}

func NewChunkManager(dirSrc storage.Storage, peerListProvider peers.Provider, peerWaitTimeout time.Duration, chunkSize int64, maxFileCountPerChunk int) (*ChunkManager, error) {
	manifest, chunks, err := newManifest(dirSrc, peerListProvider, peerWaitTimeout, chunkSize, maxFileCountPerChunk)
	if err != nil {
		return nil, err
	}

	manifestJson, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}

	return &ChunkManager{
		manifest:       manifest,
		manifestJson:   manifestJson,
		chunks:         chunks,
		chunkChecksums: make([]uint64, len(chunks)),
	}, nil
}

func (chunkManager *ChunkManager) GetManifestJson() []byte {
	return chunkManager.manifestJson
}

func (chunkManager *ChunkManager) GetManifestChecksum() uint64 {
	return chunkManager.manifest.ChunkChecksum
}

func (chunkManager *ChunkManager) WriteChunk(storage storage.Storage, chunkIndex int, reader io.Reader, expectedChecksum uint64, onFileComplete func(path string, size int64)) (int64, error) {
	var hash *xxh3.Hasher
	actualReader := reader

	if expectedChecksum != 0 {
		hash = xxh3.New()
		actualReader = io.TeeReader(reader, hash)
	}

	var length int64
	for _, fileChunk := range chunkManager.chunks[chunkIndex].FileRanges {
		file := &chunkManager.manifest.Files[fileChunk.FileIndex]
		n, err := storage.Write(file.Name, fileChunk.StartOffset, fileChunk.EndOffset-fileChunk.StartOffset, actualReader)
		if err != nil {
			return 0, err
		}
		length += n

		if file.DecrementRemainingChunkCount() == 0 {
			path := filepath.Join(storage.GetBaseDirectory(), file.Name)
			onFileComplete(path, file.Size)
		}
	}

	if hash != nil && hash.Sum64() != expectedChecksum {
		return 0, fmt.Errorf("checksum mismatch")
	}

	return length, nil
}

func (chunkManager *ChunkManager) GetChunkReader(storage storage.Storage, chunkIndex int) io.ReadCloser {
	return newChunkReadCloser(chunkManager, storage, chunkIndex)
}

func (chunkManager *ChunkManager) ComputeChunkChecksum(storage storage.Storage, chunkIndex int) (uint64, error) {
	// Fast path: return cached checksum
	checksum := atomic.LoadUint64(&chunkManager.chunkChecksums[chunkIndex])
	if checksum != 0 {
		return checksum, nil
	}

	// singleflight only deduplicates concurrent in-flight requests.
	// Once complete, the key is removed, so a double-check is needed inside.
	result, err, _ := chunkManager.chunkChecksumGroup.Do(strconv.Itoa(chunkIndex),
		func() (interface{}, error) {
			// Double-check
			checksum := atomic.LoadUint64(&chunkManager.chunkChecksums[chunkIndex])
			if checksum != 0 {
				return checksum, nil
			}

			reader := chunkManager.GetChunkReader(storage, chunkIndex)
			defer reader.Close()

			hash := xxh3.New()
			_, err := io.Copy(hash, reader)
			if err != nil {
				return uint64(0), err
			}

			checksum = hash.Sum64()
			atomic.StoreUint64(&chunkManager.chunkChecksums[chunkIndex], checksum)
			return checksum, nil
		})
	if err != nil {
		return 0, err
	}

	return result.(uint64), nil
}

func (chunkManager *ChunkManager) GetChunkChecksum(chunkIndex int) uint64 {
	return atomic.LoadUint64(&chunkManager.chunkChecksums[chunkIndex])
}

func (chunkManager *ChunkManager) SetChunkChecksum(chunkIndex int, checksum uint64) {
	atomic.StoreUint64(&chunkManager.chunkChecksums[chunkIndex], checksum)
}

func (chunkManager *ChunkManager) GetFileCount() int {
	return len(chunkManager.manifest.Files)
}

func (chunkManager *ChunkManager) GetChunkCount() int {
	return len(chunkManager.chunks)
}

func (chunkManager *ChunkManager) GetStatusDoneChunks(updatedSince time.Time) []int {
	indexList := make([]int, 0)

	for index := range chunkManager.chunks {
		if chunkManager.chunks[index].GetStatus() == types.ChunkStatusDone &&
			chunkManager.chunks[index].GetUpdatedAt().After(updatedSince) {
			indexList = append(indexList, index)
		}
	}

	return indexList
}

func (chunkManager *ChunkManager) GetChunkSize(chunkIndex int) int64 {
	return chunkManager.chunks[chunkIndex].Size()
}

func (chunkManager *ChunkManager) GetChunkStatus(chunkIndex int) types.ChunkStatus {
	return chunkManager.chunks[chunkIndex].GetStatus()
}

func (chunkManager *ChunkManager) SetChunkStatusDone(chunkIndex int, updatedAt time.Time) {
	chunkManager.chunks[chunkIndex].SetStatus(types.ChunkStatusDone)
	chunkManager.chunks[chunkIndex].SetUpdatedAt(updatedAt)
}

func (chunkManager *ChunkManager) SetChunkStatusPending(chunkIndex int) {
	chunkManager.chunks[chunkIndex].SetStatus(types.ChunkStatusPending)
}

func (chunkManager *ChunkManager) UpdateChunkStatus(chunkIndex int, oldStatus, newStatus types.ChunkStatus) bool {
	return chunkManager.chunks[chunkIndex].UpdateStatus(oldStatus, newStatus)
}
