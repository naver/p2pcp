// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"reflect"
	"sort"
	"testing"
)

func TestManifest_createTransferChunks(t *testing.T) {
	type fields struct {
		ChunkSize int64
		Files     []File
	}
	tests := []struct {
		name   string
		fields fields
		want   []TransferChunk
	}{
		{
			"test 1",
			fields{
				ChunkSize: 1000,
				Files: []File{
					{Size: 500},
					{Size: 2000},
					{Size: 500},
				},
			},
			[]TransferChunk{
				{
					FileChunks: []FileChunk{
						{0, 0, 500},
						{1, 0, 500},
					},
				},
				{
					FileChunks: []FileChunk{
						{1, 500, 1500},
					},
				},
				{
					FileChunks: []FileChunk{
						{1, 1500, 2000},
						{2, 0, 500},
					},
				},
			},
		},
		{
			"test 2",
			fields{
				ChunkSize: 1000,
				Files: []File{
					{Size: 2000},
					{Size: 500},
					{Size: 2000},
				},
			},
			[]TransferChunk{
				{
					FileChunks: []FileChunk{
						{0, 0, 1000},
					},
				},
				{
					FileChunks: []FileChunk{
						{0, 1000, 2000},
					},
				},
				{
					FileChunks: []FileChunk{
						{1, 0, 500},
						{2, 0, 500},
					},
				},
				{
					FileChunks: []FileChunk{
						{2, 500, 1500},
					},
				},
				{
					FileChunks: []FileChunk{
						{2, 1500, 2000},
					},
				},
			},
		},
		{
			"test 3",
			fields{
				ChunkSize: 1000,
				Files: []File{
					{Size: 300},
					{Size: 300},
					{Size: 300},
					{Size: 300},
					{Size: 300},
				},
			},
			[]TransferChunk{
				{
					FileChunks: []FileChunk{
						{0, 0, 300},
						{1, 0, 300},
						{2, 0, 300},
						{3, 0, 100},
					},
				},
				{
					FileChunks: []FileChunk{
						{3, 100, 300},
						{4, 0, 300},
					},
				},
			},
		},
		{

			"test 4",
			fields{
				ChunkSize: 300,
				Files: []File{
					{Size: 2000},
					{Size: 500},
					{Size: 1000},
				},
			},
			[]TransferChunk{
				{
					FileChunks: []FileChunk{
						{0, 0, 300},
					},
				},
				{
					FileChunks: []FileChunk{
						{0, 300, 600},
					},
				},
				{
					FileChunks: []FileChunk{
						{0, 600, 900},
					},
				},
				{
					FileChunks: []FileChunk{
						{0, 900, 1200},
					},
				},
				{
					FileChunks: []FileChunk{
						{0, 1200, 1500},
					},
				},
				{
					FileChunks: []FileChunk{
						{0, 1500, 1800},
					},
				},
				{
					FileChunks: []FileChunk{
						{0, 1800, 2000},
						{1, 0, 100},
					},
				},
				{
					FileChunks: []FileChunk{
						{1, 100, 400},
					},
				},
				{
					FileChunks: []FileChunk{
						{1, 400, 500},
						{2, 0, 200},
					},
				},
				{
					FileChunks: []FileChunk{
						{2, 200, 500},
					},
				},
				{
					FileChunks: []FileChunk{
						{2, 500, 800},
					},
				},
				{
					FileChunks: []FileChunk{
						{2, 800, 1000},
					},
				},
			},
		},
		{
			"MaxFileCountPerChunk test 1",
			fields{
				ChunkSize: 1000,
				Files: []File{
					{Size: 10},
					{Size: 10},
					{Size: 10},
					{Size: 10},
					{Size: 10},
					{Size: 10},
					{Size: 10},
					{Size: 10},
					{Size: 10},
					{Size: 10},
					{Size: 10},
					{Size: 10},
				},
			},
			[]TransferChunk{
				{
					FileChunks: []FileChunk{
						{0, 0, 10},
						{1, 0, 10},
						{2, 0, 10},
						{3, 0, 10},
						{4, 0, 10},
					},
				},
				{
					FileChunks: []FileChunk{
						{5, 0, 10},
						{6, 0, 10},
						{7, 0, 10},
						{8, 0, 10},
						{9, 0, 10},
					},
				},
				{
					FileChunks: []FileChunk{
						{10, 0, 10},
						{11, 0, 10},
					},
				},
			},
		},
		{
			"MaxFileCountPerChunk test 2",
			fields{
				ChunkSize: 1000,
				Files: []File{
					{Size: 10},
					{Size: 10},
					{Size: 10},
					{Size: 1000},
					{Size: 10},
					{Size: 10},
					{Size: 10},
					{Size: 1000},
					{Size: 10},
					{Size: 10},
					{Size: 10},
					{Size: 910},
					{Size: 10},
					{Size: 10},
					{Size: 10},
					{Size: 10},
					{Size: 10},
					{Size: 10},
				},
			},
			[]TransferChunk{
				{
					FileChunks: []FileChunk{
						{0, 0, 10},
						{1, 0, 10},
						{2, 0, 10},
						{3, 0, 970},
					},
				},
				{
					FileChunks: []FileChunk{
						{3, 970, 1000},
						{4, 0, 10},
						{5, 0, 10},
						{6, 0, 10},
						{7, 0, 940},
					},
				},
				{
					FileChunks: []FileChunk{
						{7, 940, 1000},
						{8, 0, 10},
						{9, 0, 10},
						{10, 0, 10},
						{11, 0, 910},
					},
				},
				{
					FileChunks: []FileChunk{
						{12, 0, 10},
						{13, 0, 10},
						{14, 0, 10},
						{15, 0, 10},
						{16, 0, 10},
					},
				},
				{
					FileChunks: []FileChunk{
						{17, 0, 10},
					},
				},
			},
		},
	}
	*flagMaxFileCountPerChunk = 5
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manifest{
				ChunkSize: tt.fields.ChunkSize,
				Files:     tt.fields.Files,
			}

			sort.Slice(m.Files, func(i, j int) bool {
				return m.Files[i].Name < m.Files[j].Name
			})
			if got := m.createTransferChunks(m.Files); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Manifest.createTransferChunks() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestManifest_createChunkChecksum(t *testing.T) {
	m := &Manifest{}

	base_files := []File{
		{Name: "a", Size: 10, Perm: 644},
		{Name: "b", Size: 20, Perm: 600},
	}

	base_chunk := []TransferChunk{
		{
			FileChunks: []FileChunk{
				{0, 0, 300},
				{1, 0, 300},
				{2, 0, 300},
				{3, 0, 100},
			},
		},
		{
			FileChunks: []FileChunk{
				{3, 100, 300},
				{4, 0, 300},
			},
		},
	}
	last_checksum := ""

	t.Run("test 1", func(t *testing.T) {
		got, err := m.createChunkChecksum(base_files, base_chunk)
		if err != nil {
			t.Errorf("Manifest.createChunkChecksum() error = %v", err)
			return
		}
		if got == last_checksum {
			t.Errorf("Manifest.createChunkChecksum() = %v, want %v", got, last_checksum)
		}
		last_checksum = got
	})

	base_chunk[1].FileChunks[0], base_chunk[1].FileChunks[1] = base_chunk[1].FileChunks[1], base_chunk[1].FileChunks[0]
	t.Run("test 2", func(t *testing.T) {
		got, err := m.createChunkChecksum(base_files, base_chunk)
		if err != nil {
			t.Errorf("Manifest.createChunkChecksum() error = %v", err)
			return
		}
		if got == last_checksum {
			t.Errorf("Manifest.createChunkChecksum() = %v, want %v", got, last_checksum)
		}
		last_checksum = got
	})

	base_chunk[0].FileChunks[0].ToOffset = 1
	t.Run("test 3", func(t *testing.T) {
		got, err := m.createChunkChecksum(base_files, base_chunk)
		if err != nil {
			t.Errorf("Manifest.createChunkChecksum() error = %v", err)
			return
		}
		if got == last_checksum {
			t.Errorf("Manifest.createChunkChecksum() = %v, want %v", got, last_checksum)
		}
		last_checksum = got
	})

	base_chunk[1].FileChunks = append(base_chunk[1].FileChunks, FileChunk{})
	t.Run("test 4", func(t *testing.T) {
		got, err := m.createChunkChecksum(base_files, base_chunk)
		if err != nil {
			t.Errorf("Manifest.createChunkChecksum() error = %v", err)
			return
		}
		if got == last_checksum {
			t.Errorf("Manifest.createChunkChecksum() = %v, want %v", got, last_checksum)
		}
		last_checksum = got
	})

	base_files[0].Perm = 777
	t.Run("test 5", func(t *testing.T) {
		got, err := m.createChunkChecksum(base_files, base_chunk)
		if err != nil {
			t.Errorf("Manifest.createChunkChecksum() error = %v", err)
			return
		}
		if got == last_checksum {
			t.Errorf("Manifest.createChunkChecksum() = %v, want %v", got, last_checksum)
		}
		last_checksum = got
	})
}
