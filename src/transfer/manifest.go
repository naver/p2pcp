// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package transfer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/naver/p2pcp/constants"
	p2pcpHttp "github.com/naver/p2pcp/http"
	"github.com/naver/p2pcp/peers"
	"github.com/naver/p2pcp/storage"
	"github.com/naver/p2pcp/types"
	"github.com/naver/p2pcp/utils"
	"github.com/naver/p2pcp/version"
	"github.com/zeebo/xxh3"
)

type Manifest struct {
	// Version of p2pcp, communication guaranteed between same major versions
	MajorVersion string `json:"majorVersion"`
	MinorVersion string `json:"minorVersion"`

	// Size to use when generating chunks from the list of files to transfer
	ChunkSize int64 `json:"chunkSize"`

	// Checksum for the chunk list
	// Each p2pcp generates a chunk list from the file list and compares it to ensure identical chunk lists
	ChunkChecksum      uint64 `json:"checksum"`
	TransferChunkCount int    `json:"transferChunkCount"`

	Dirs       []types.DirEntry       `json:"dirs"`
	Symlinks   []types.SymlinkEntry   `json:"symlinks"`
	EmptyFiles []types.EmptyFileEntry `json:"emptyFiles"`
	Files      []types.FileEntry      `json:"files"`
}

func newManifest(dirSrc storage.Storage, peerListProvider peers.Provider, peerWaitTimeout time.Duration, chunkSize int64, maxFileCountPerChunk int) (*Manifest, []types.Chunk, error) {
	manifest := &Manifest{
		MajorVersion: version.MajorVersion,
		MinorVersion: version.MinorVersion,
		ChunkSize:    chunkSize,
	}

	var chunks []types.Chunk
	var err error

	if dirSrc != nil {
		utils.DebugPrintf("Making files list...")

		chunks, err = manifest.getManifestFromStorage(dirSrc, maxFileCountPerChunk)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create a file list from the source storage: %s", err.Error())
		}
	} else if peerListProvider != nil {
		utils.DebugPrintf("Waiting peer who has complete files list...")

		chunks, err = manifest.getManifestFromPeerList(peerListProvider, peerWaitTimeout, maxFileCountPerChunk)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create a file list from the peer list: %s", err.Error())
		}
	} else {
		return nil, nil, errors.New("at least one of local source directory or peer list required")
	}

	return manifest, chunks, nil
}

func (manifest *Manifest) getManifestFromPeer(peerHost string, maxFileCountPerChunk int) ([]types.Chunk, error) {
	content, err := p2pcpHttp.Request(
		fmt.Sprintf("http://%s/manifest", peerHost),
		p2pcpHttp.WithExpectedContentType("application/json"),
	)
	if err != nil {
		return nil, err
	}

	var receivedManifest Manifest
	err = json.Unmarshal(content, &receivedManifest)
	if err != nil {
		return nil, err
	}

	if receivedManifest.MajorVersion != manifest.MajorVersion {
		return nil, fmt.Errorf("peer %s has different major version (expected=%s.%s, actual=%s.%s)", peerHost, manifest.MajorVersion, manifest.MinorVersion, receivedManifest.MajorVersion, receivedManifest.MinorVersion)
	}

	if receivedManifest.ChunkSize != manifest.ChunkSize {
		return nil, fmt.Errorf("peer %s has different chunk size (expected=%d, actual=%d)", peerHost, manifest.ChunkSize, receivedManifest.ChunkSize)
	}

	chunks := types.BuildChunks(receivedManifest.Files, manifest.ChunkSize, maxFileCountPerChunk)
	checksum, err := receivedManifest.computeChecksum(chunks)
	if err != nil {
		return nil, fmt.Errorf("failed to compute checksum for the received manifest: %s", err.Error())
	}

	if receivedManifest.ChunkChecksum != checksum {
		return nil, fmt.Errorf("peer %s has different checksum (expected=%d, actual=%d)", peerHost, checksum, receivedManifest.ChunkChecksum)
	}

	transferChunkCount := len(chunks)
	if receivedManifest.TransferChunkCount != transferChunkCount {
		return nil, fmt.Errorf("peer %v has different chunk count: expected=%v, actual=%v", peerHost, transferChunkCount, receivedManifest.TransferChunkCount)
	}

	manifest.ChunkChecksum = receivedManifest.ChunkChecksum
	manifest.TransferChunkCount = receivedManifest.TransferChunkCount
	manifest.Dirs = receivedManifest.Dirs
	manifest.Symlinks = receivedManifest.Symlinks
	manifest.EmptyFiles = receivedManifest.EmptyFiles
	manifest.Files = receivedManifest.Files

	return chunks, nil
}

func (manifest *Manifest) getManifestFromStorage(storage storage.Storage, maxFileCountPerChunk int) ([]types.Chunk, error) {
	manifest.Dirs = make([]types.DirEntry, 0)
	manifest.Symlinks = make([]types.SymlinkEntry, 0)
	manifest.EmptyFiles = make([]types.EmptyFileEntry, 0)
	manifest.Files = make([]types.FileEntry, 0)

	err := storage.Walk("", func(entry types.Entry) error {
		switch e := entry.(type) {
		case *types.DirEntry:
			manifest.Dirs = append(manifest.Dirs, *e)
		case *types.SymlinkEntry:
			manifest.Symlinks = append(manifest.Symlinks, *e)
		case *types.EmptyFileEntry:
			manifest.EmptyFiles = append(manifest.EmptyFiles, *e)
		case *types.FileEntry:
			manifest.Files = append(manifest.Files, e.Copy())
		}
		return nil
	})

	chunks := types.BuildChunks(manifest.Files, manifest.ChunkSize, maxFileCountPerChunk)
	checksum, err := manifest.computeChecksum(chunks)
	if err != nil {
		return nil, fmt.Errorf("failed to compute checksum for the manifest: %s", err.Error())
	}

	manifest.ChunkChecksum = checksum
	manifest.TransferChunkCount = len(chunks)

	return chunks, nil
}

func (manifest *Manifest) getManifestFromPeerList(peerListProvider peers.Provider, peerWaitTimeout time.Duration, maxFileCountPerChunk int) ([]types.Chunk, error) {
	ctx, cancel := context.WithTimeout(context.Background(), peerWaitTimeout)
	defer cancel()

	for {
		startTimestamp := time.Now()

		peerList, err := peerListProvider.GetPeers(ctx)
		if err != nil {
			utils.DebugPrintf("Failed to getting peer list from provider.: %s", err.Error())
		} else {
			for _, peer := range peerList {
				chunks, err := manifest.getManifestFromPeer(peer, maxFileCountPerChunk)
				if err == nil {
					return chunks, nil
				}
				utils.DebugPrintf("Failed to getting manifest from peer.: (%s) %s", peer, err.Error())
			}
		}

		sleepDuration := constants.PeerWaitDuration - time.Since(startTimestamp)
		if sleepDuration < 0 {
			sleepDuration = 0
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("failed to get manifest from peer: %s", ctx.Err().Error())
		case <-time.After(sleepDuration):
		}
	}
}

func (manifest *Manifest) computeChecksum(chunks []types.Chunk) (uint64, error) {
	hash := xxh3.New()

	jsonData, err := json.Marshal(manifest.Files)
	if err != nil {
		return 0, err
	}
	hash.Write(jsonData)

	jsonData, err = json.Marshal(chunks)
	if err != nil {
		return 0, err
	}
	hash.Write(jsonData)

	return hash.Sum64(), nil
}
