// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type FileDownloader interface {
	Name() string
	RequestDownload(transferChunkIndex int) (io.ReadCloser, error)
	FlushAvailableList(queue *ChunkQueue)
	IsValid() bool
	Fin()
}

type localFileReadCloser struct {
	dirSrc         string
	files          *[]File
	fileChunks     *[]FileChunk
	fileChunkIndex int
	readCloser     io.ReadCloser
}

func (l *localFileReadCloser) Close() error {
	if l.readCloser != nil {
		return l.readCloser.Close()
	}
	return nil
}

func (l *localFileReadCloser) Read(p []byte) (int, error) {
	var length int

	for ; l.fileChunkIndex < len(*l.fileChunks); l.fileChunkIndex++ {
		fileChunk := (*l.fileChunks)[l.fileChunkIndex]
		file := (*l.files)[fileChunk.FileIndex]

		if l.readCloser == nil {
			f, err := os.Open(l.dirSrc + file.Name)
			if err != nil {
				return length, err
			}

			_, err = f.Seek(fileChunk.FromOffset, io.SeekStart)
			if err != nil {
				return length, err
			}

			l.readCloser = LimitReadCloser(f, fileChunk.ToOffset-fileChunk.FromOffset)
		}

		n, err := l.readCloser.Read(p[length:])
		if err == io.EOF {
			l.readCloser.Close()
			l.readCloser = nil
			length += n
		} else if err != nil {
			return length, err
		} else {
			length += n
			return length, nil
		}
	}
	return length, nil
}

func LocalFileReadCloser(mainContext *MainContext, transferChunkIndex int) io.ReadCloser {
	return &localFileReadCloser{
		dirSrc:         mainContext.DirSrc,
		files:          &mainContext.Manifest.Files,
		fileChunks:     &mainContext.TransferChunks[transferChunkIndex].FileChunks,
		fileChunkIndex: 0,
		readCloser:     nil,
	}
}

type LocalFileDownloader struct {
	mainContext   *MainContext
	availableList []int
}

func (downloader *LocalFileDownloader) IsValid() bool {
	return true
}

func (downloader *LocalFileDownloader) Fin() {
}

func (downloader *LocalFileDownloader) RequestDownload(transferChunkIndex int) (io.ReadCloser, error) {
	return LocalFileReadCloser(downloader.mainContext, transferChunkIndex), nil
}

func (downloader *LocalFileDownloader) Name() string {
	return "Local"
}

func (downloader *LocalFileDownloader) Init(mainContext *MainContext) {
	downloader.mainContext = mainContext

	/* 전송해야 하는 transferChunkIndex 목록 */
	downloader.availableList = make([]int, mainContext.Manifest.TransferChunkCount)
	for i := range downloader.availableList {
		downloader.availableList[i] = i
	}
}

func (downloader *LocalFileDownloader) FlushAvailableList(queue *ChunkQueue) {
	for i := 0; i < len(downloader.availableList); i++ {
		queue.PushShuffle(downloader.availableList[i])
	}
	downloader.availableList = make([]int, 0)
}

const (
	AvaliableChunkStatusNotReady = iota
	AvaliableChunkStatusReady
	AvaliableChunkStatusQueued
)

type PeerFileDownloader struct {
	mainContext *MainContext
	peerHost    string
	// chunk index 에 해당하는 chunk 가 어떤 상태인지 지정 (AvaliableChunkStatus...)
	availableList      []int
	availableListCount int
	availableListMutex sync.Mutex
	lastUpdatedAt      time.Time
	isSelf             atomic.Bool
	wantStopUpdate     chan bool
	wgUpdate           sync.WaitGroup
	httpClient         *HttpClient
}

func (downloader *PeerFileDownloader) RequestDownload(transferChunkIndex int) (io.ReadCloser, error) {
	url := fmt.Sprintf("http://%v/chunk/%v", downloader.peerHost, transferChunkIndex)
	options := NewHttpRequestOptions()
	options.EncodingType = *flagCompressType
	options.ExpectedContentType = "application/octet-stream"
	return downloader.httpClient.RequestHttpAsync(url, options)
}

func (downloader *PeerFileDownloader) Name() string {
	return "Peer:" + downloader.peerHost
}

func (downloader *PeerFileDownloader) updateAvailableList() error {
	params := url.Values{}
	params.Add("only_updated_since", downloader.lastUpdatedAt.Format(time.RFC3339Nano))
	url := fmt.Sprintf("http://%v/chunk?%v", downloader.peerHost, params.Encode())
	options := NewHttpRequestOptions()
	options.EncodingType = *flagCompressType
	options.ExpectedContentType = "application/json"
	content, err := downloader.httpClient.RequestHttp(url, options)
	if err != nil {
		return err
	}

	var resultObject AvailableChunkList
	err = json.Unmarshal(content, &resultObject)
	if err != nil {
		return err
	}

	timestamp, err := time.Parse(time.RFC3339Nano, resultObject.Timestamp)
	if err != nil {
		return err
	}

	downloader.lastUpdatedAt = timestamp

	downloader.availableListMutex.Lock()
	if resultObject.IsCompleted {
		for i := 0; i < mainContext.Manifest.TransferChunkCount; i++ {
			if downloader.availableList[i] == AvaliableChunkStatusNotReady {
				downloader.availableList[i] = AvaliableChunkStatusReady
				downloader.availableListCount++
			}
		}
	} else {
		for _, i := range resultObject.TransferChunkIndexList {
			if downloader.availableList[i] == AvaliableChunkStatusNotReady {
				downloader.availableList[i] = AvaliableChunkStatusReady
				downloader.availableListCount++
			}
		}
	}
	downloader.availableListMutex.Unlock()

	return nil
}

func (downloader *PeerFileDownloader) IsValid() bool {
	return !downloader.isSelf.Load()
}

func (downloader *PeerFileDownloader) setIsSelf() error {
	url := fmt.Sprintf("http://%v/uuid", downloader.peerHost)
	options := NewHttpRequestOptions()
	options.ExpectedContentType = "text/plain"
	content, err := downloader.httpClient.RequestHttp(url, options)
	if err != nil {
		return err
	}

	peerUUID := strings.TrimSpace(string(content))
	downloader.isSelf.Store(downloader.mainContext.UUID == peerUUID)

	return nil
}

func (downloader *PeerFileDownloader) verifyChecksum() (bool, error) {
	url := fmt.Sprintf("http://%v/manifest/checksum", downloader.peerHost)
	options := NewHttpRequestOptions()
	options.ExpectedContentType = "text/plain"
	content, err := downloader.httpClient.RequestHttp(url, options)
	if err != nil {
		return false, err
	}

	checksum := strings.TrimSpace(string(content))
	return mainContext.Manifest.ChunkChecksum == checksum, nil
}

func (downloader *PeerFileDownloader) Fin() {
	downloader.wantStopUpdate <- true
	downloader.wgUpdate.Wait()
}

func (downloader *PeerFileDownloader) Init(mainContext *MainContext, peerHost string) {
	downloader.mainContext = mainContext
	downloader.peerHost = peerHost
	downloader.availableList = make([]int, mainContext.Manifest.TransferChunkCount)
	downloader.availableListCount = 0
	downloader.wantStopUpdate = make(chan bool, 1)
	downloader.httpClient = NewHttpClient(PeerTransferTimeout, mainContext.PeerNumConcurrent+1)
	downloader.wgUpdate.Add(1)

	go func() {
		defer downloader.wgUpdate.Done()

		ticker := time.NewTicker(*flagAvailableListSyncInterval)
		defer ticker.Stop()

		var err error
		var isSelfInit bool
		var isValidInit bool
		for {
			/* 제공된 peer 목록에서 자기자신이 포함되어 있을 수 있으므로 체크 */
			if !isSelfInit {
				err := downloader.setIsSelf()
				if err != nil {
					ErrorPrintf("Failed to update isSelf %s: %s", peerHost, err.Error())
					goto WAIT
				}
				isSelfInit = true

				if downloader.isSelf.Load() {
					DebugPrintf("Peer %v is this server itself.", peerHost)
					break
				}
			}
			if !isValidInit {
				isValid, err := downloader.verifyChecksum()
				if err != nil {
					ErrorPrintf("Failed to verify checksum %s: %s", peerHost, err.Error())
					goto WAIT
				}
				isValidInit = true

				if !isValid {
					ErrorPrintf("Failed to verify checksum %s: peer has different checksum", peerHost)
					mainContext.WantStopDownload.Store(true)
					return
				}
			}

			err = downloader.updateAvailableList()
			if err != nil {
				ErrorPrintf("Failed to receive available list from peer %s: %s", peerHost, err.Error())
			} else if downloader.availableListCount == mainContext.Manifest.TransferChunkCount {
				DebugPrintf("Peer %v now has complete available list.", peerHost)
				break
			}

		WAIT:
			select {
			case <-ticker.C:
			case <-downloader.wantStopUpdate:
				return
			}
		}
	}()
}

func (downloader *PeerFileDownloader) FlushAvailableList(queue *ChunkQueue) {
	downloader.availableListMutex.Lock()
	for i := 0; i < mainContext.Manifest.TransferChunkCount; i++ {
		if downloader.availableList[i] == AvaliableChunkStatusReady {
			downloader.availableList[i] = AvaliableChunkStatusQueued
			queue.PushShuffle(i)
		}
	}
	downloader.availableListMutex.Unlock()
}
