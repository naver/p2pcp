// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package types

import (
	"io/fs"
	"sync/atomic"
)

type Entry interface {
	GetName() string
}

type DirEntry struct {
	Name string      `json:"name"`
	Perm fs.FileMode `json:"perm"`
}

func (e *DirEntry) GetName() string {
	return e.Name
}

type SymlinkEntry struct {
	Name      string `json:"name"`
	SymlinkTo string `json:"symlinkTo"`
}

func (e *SymlinkEntry) GetName() string {
	return e.Name
}

type EmptyFileEntry struct {
	Name string      `json:"name"`
	Perm fs.FileMode `json:"perm"`
}

func (e *EmptyFileEntry) GetName() string {
	return e.Name
}

type FileEntry struct {
	Name                string       `json:"name"`
	Size                int64        `json:"size"`
	Perm                fs.FileMode  `json:"perm"`
	remainingChunkCount atomic.Int32 `json:"-"`
}

func (e *FileEntry) GetName() string {
	return e.Name
}

func (e *FileEntry) Copy() FileEntry {
	return FileEntry{
		Name: e.Name,
		Size: e.Size,
		Perm: e.Perm,
	}
}

func (e *FileEntry) IncrementRemainingChunkCount() int32 {
	return e.remainingChunkCount.Add(1)
}

func (e *FileEntry) DecrementRemainingChunkCount() int32 {
	return e.remainingChunkCount.Add(-1)
}
