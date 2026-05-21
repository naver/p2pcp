// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/naver/p2pcp/constants"
	p2pcpHttp "github.com/naver/p2pcp/http"
	"github.com/naver/p2pcp/transfer"
	"github.com/naver/p2pcp/utils"
)

const (
	AvailableChunkNotReady = iota
	AvailableChunkReady
)

type PeerDownloader struct {
	ChunkManager        *transfer.ChunkManager
	Queue               *transfer.ChunkQueue
	UUID                string
	Host                string
	AvailableChunkList  []int
	AvailableChunkCount int
	HttpClient          *p2pcpHttp.HttpClient
	LastUpdatedAt       time.Time
	IsSelf              atomic.Bool

	updateCtx    context.Context
	cancelUpdate context.CancelFunc
	updateDone   chan struct{}
}

func NewPeerDownloader(mainContext *MainContext, host string, numPeerConcurrent int) *PeerDownloader {
	updateCtx, cancelUpdate := context.WithCancel(mainContext.DownloadCtx)
	downloader := &PeerDownloader{
		ChunkManager:        mainContext.ChunkManager,
		Queue:               transfer.NewChunkQueue(),
		UUID:                mainContext.UUID,
		Host:                host,
		AvailableChunkList:  make([]int, mainContext.ChunkManager.GetChunkCount()),
		AvailableChunkCount: 0,
		HttpClient:          p2pcpHttp.NewHttpClient(numPeerConcurrent + 1),

		updateCtx:    updateCtx,
		cancelUpdate: cancelUpdate,
		updateDone:   make(chan struct{}),
	}

	go func() {
		defer close(downloader.updateDone)

		ticker := time.NewTicker(*flagAvailableListSyncInterval)
		defer ticker.Stop()

		var isSelfInit bool
		var isValidInit bool
		for {
			func() {
				// The peer list may include oneself;
				// there's no need to update the AvailableList from oneself, so exit the loop.
				if !isSelfInit {
					err := downloader.setIsSelf()
					if err != nil {
						utils.ErrorPrintf("Failed to update IsSelf.: (%s) %s", downloader.Host, err.Error())
						return
					}
					if downloader.IsSelf.Load() {
						utils.DebugPrintf("Peer %s is this server itself.", downloader.Host)
						downloader.cancelUpdate()
						return
					}
					isSelfInit = true
				}
				if !isValidInit {
					isValid, err := downloader.verifyChecksum()
					if err != nil {
						utils.ErrorPrintf("Failed to get checksum.: (%s) %s", downloader.Host, err.Error())
						return
					}
					if !isValid {
						utils.ErrorPrintf("Failed to verify checksum.: (%s) peer has different checksum", downloader.Host)
						downloader.cancelUpdate()
						mainContext.ExitCode.Store(1)
						mainContext.DownloadCancel()
						return
					}
					isValidInit = true
				}

				err := downloader.UpdatePeerAvailableList()
				if err != nil {
					utils.ErrorPrintf("Failed to receive available list from peer.: (%s) %s", downloader.Host, err.Error())
					return
				}

				if downloader.AvailableChunkCount == downloader.ChunkManager.GetChunkCount() {
					utils.DebugPrintf("Peer %s now has complete available list.", downloader.Host)
					downloader.cancelUpdate()
					return
				}
			}()

			select {
			case <-ticker.C:
			case <-downloader.updateCtx.Done():
				return
			}
		}
	}()

	return downloader
}

func (downloader *PeerDownloader) setIsSelf() error {
	content, err := downloader.HttpClient.RequestBody(
		fmt.Sprintf("http://%s/uuid", downloader.Host),
		p2pcpHttp.WithExpectedContentType("text/plain"),
	)
	if err != nil {
		return err
	}

	peerUUID := strings.TrimSpace(string(content))
	downloader.IsSelf.Store(downloader.UUID == peerUUID)

	return nil
}

func (downloader *PeerDownloader) verifyChecksum() (bool, error) {
	content, err := downloader.HttpClient.RequestBody(
		fmt.Sprintf("http://%s/manifest/checksum", downloader.Host),
		p2pcpHttp.WithExpectedContentType("text/plain"),
	)
	if err != nil {
		return false, err
	}

	checksum, err := strconv.ParseUint(string(content), 10, 64)
	if err != nil {
		return false, err
	}
	return downloader.ChunkManager.GetManifestChecksum() == checksum, nil
}

func (downloader *PeerDownloader) Name() string {
	return "Peer:" + downloader.Host
}

func (downloader *PeerDownloader) GetQueue() *transfer.ChunkQueue {
	return downloader.Queue
}

func (downloader *PeerDownloader) IsValid() bool {
	return !downloader.IsSelf.Load()
}

func (downloader *PeerDownloader) Fin() {
	downloader.cancelUpdate()
	<-downloader.updateDone

	// If the peer downloader is finished, it is no longer valid.
	downloader.IsSelf.Store(true)
}

func (downloader *PeerDownloader) RequestDownload(chunkIndex int, withChecksum bool) (uint64, io.ReadCloser, error) {
	url := fmt.Sprintf("http://%v/chunk/%v", downloader.Host, chunkIndex)
	options := []p2pcpHttp.Option{
		p2pcpHttp.WithAcceptEncodingType(*flagCompressType),
		p2pcpHttp.WithExpectedContentType("application/octet-stream"),
	}

	if withChecksum {
		options = append(options, p2pcpHttp.WithWantDigest(constants.DefaultDigest))
		header, readCloser, err := downloader.HttpClient.Request(url, options...)
		if err != nil {
			return 0, nil, err
		}

		checksum, err := ParseDigest(header)
		return checksum, readCloser, err
	} else {
		_, readCloser, err := downloader.HttpClient.Request(url, options...)
		return 0, readCloser, err
	}
}

func (downloader *PeerDownloader) UpdatePeerAvailableList() error {
	params := url.Values{}
	params.Add("only_updated_since", downloader.LastUpdatedAt.Format(time.RFC3339Nano))

	content, err := downloader.HttpClient.RequestBody(
		fmt.Sprintf("http://%s/chunk?%s", downloader.Host, params.Encode()),
		p2pcpHttp.WithAcceptEncodingType(*flagCompressType),
		p2pcpHttp.WithExpectedContentType("application/json"),
	)
	if err != nil {
		return err
	}

	var availableList AvailableList
	if err := json.Unmarshal(content, &availableList); err != nil {
		return err
	}
	downloader.LastUpdatedAt = availableList.Timestamp

	indexList := []int{}
	if availableList.IsCompleted {
		for chunkIndex := range downloader.ChunkManager.GetChunkCount() {
			if downloader.markChunkAsReady(chunkIndex) {
				indexList = append(indexList, chunkIndex)
			}
		}
	} else {
		for _, chunkIndex := range availableList.TransferChunkIndexList {
			if downloader.markChunkAsReady(chunkIndex) {
				indexList = append(indexList, chunkIndex)
			}
		}
	}
	downloader.Queue.PushShuffledList(indexList)

	return nil
}

func (downloader *PeerDownloader) markChunkAsReady(chunkIndex int) bool {
	if downloader.AvailableChunkList[chunkIndex] == AvailableChunkNotReady {
		downloader.AvailableChunkList[chunkIndex] = AvailableChunkReady
		downloader.AvailableChunkCount++
		return true
	}
	return false
}
