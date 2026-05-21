// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package transfer

import (
	"io"

	"github.com/naver/p2pcp/storage"
)

type chunkReadCloser struct {
	chunkManager   *ChunkManager
	storage        storage.Storage
	chunkIndex     int
	fileRangeIndex int
	readCloser     io.ReadCloser
}

func newChunkReadCloser(chunkManager *ChunkManager, storage storage.Storage, chunkIndex int) io.ReadCloser {
	return &chunkReadCloser{
		chunkManager:   chunkManager,
		storage:        storage,
		chunkIndex:     chunkIndex,
		fileRangeIndex: 0,
		readCloser:     nil,
	}
}

func (reader *chunkReadCloser) Read(p []byte) (int, error) {
	if reader.readCloser == nil {
		fileRanges := reader.chunkManager.chunks[reader.chunkIndex].FileRanges
		if reader.fileRangeIndex < 0 || reader.fileRangeIndex >= len(fileRanges) {
			return 0, io.EOF
		}

		fileRange := fileRanges[reader.fileRangeIndex]
		file := &reader.chunkManager.manifest.Files[fileRange.FileIndex]
		readCloser, err := reader.storage.GetReader(file.Name, fileRange.StartOffset, fileRange.EndOffset-fileRange.StartOffset)
		if err != nil {
			return 0, err
		}
		reader.readCloser = readCloser
	}

	n, err := reader.readCloser.Read(p)
	if err != nil {
		if err == io.EOF {
			reader.readCloser.Close()
			reader.readCloser = nil
			reader.fileRangeIndex++
			return n, nil
		}
		return n, err
	}
	return n, nil
}

func (reader *chunkReadCloser) Close() error {
	if reader.readCloser != nil {
		err := reader.readCloser.Close()
		reader.readCloser = nil
		if err != nil {
			return err
		}
	}
	return nil
}
