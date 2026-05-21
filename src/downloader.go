// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/naver/p2pcp/constants"
	"github.com/naver/p2pcp/peers"
	"github.com/naver/p2pcp/transfer"
	"github.com/naver/p2pcp/types"
	"github.com/naver/p2pcp/utils"
)

type Downloader interface {
	Name() string
	GetQueue() *transfer.ChunkQueue
	RequestDownload(chunkIndex int, withChecksum bool) (uint64, io.ReadCloser, error)
	IsValid() bool
	Fin()
}

type PeerDownloaders struct {
	mainContext       *MainContext
	numPeerConcurrent int

	wg          sync.WaitGroup
	lock        sync.RWMutex
	downloaders map[string]*PeerDownloader
}

func NewPeerDownloaders(mainContext *MainContext, numPeerConcurrent int) *PeerDownloaders {
	return &PeerDownloaders{
		mainContext:       mainContext,
		numPeerConcurrent: numPeerConcurrent,
		downloaders:       make(map[string]*PeerDownloader),
	}
}

func (p *PeerDownloaders) Fin() {
	p.lock.RLock()
	remaining := len(p.downloaders)
	p.lock.RUnlock()
	if remaining != 0 {
		utils.ErrorPrintf("Peer downloaders are not finished: %d", remaining)
	}

	p.wg.Wait()
}

func (p *PeerDownloaders) Add(host string) {
	p.lock.RLock()
	_, ok := p.downloaders[host]
	p.lock.RUnlock()
	if ok {
		utils.ErrorPrintf("Peer %s already exists, skipping add", host)
		return
	}

	downloader := NewPeerDownloader(p.mainContext, host, p.numPeerConcurrent)

	p.lock.Lock()
	p.downloaders[host] = downloader
	p.lock.Unlock()
	utils.DebugPrintf("Peer added: %s", host)

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		execDownloader(p.mainContext, downloader, *flagPeerNumConcurrent)
	}()
}

func (p *PeerDownloaders) Remove(host string) {
	p.lock.RLock()
	downloader, ok := p.downloaders[host]
	p.lock.RUnlock()
	if !ok {
		utils.ErrorPrintf("Peer %s not found, skipping remove", host)
		return
	}

	p.lock.Lock()
	delete(p.downloaders, host)
	p.lock.Unlock()
	utils.DebugPrintf("Peer removed: %s", host)

	downloader.Fin()
}

func DownloadFiles(mainContext *MainContext) error {
	err := transfer.SetupTransfer(mainContext.ChunkManager, mainContext.Dst)
	if err != nil {
		return err
	}

	t0 := time.Now()

	var wg sync.WaitGroup

	if mainContext.PeerListProvider != nil {
		wg.Add(1)

		go func() {
			defer wg.Done()

			peerDownloaders := NewPeerDownloaders(mainContext, *flagPeerNumConcurrent)
			defer peerDownloaders.Fin()

			controller := peers.NewController(mainContext.DownloadCtx, mainContext.PeerListProvider, *flagPeerWaitTimeout, *flagPeerListPollInterval, peerDownloaders.Add, peerDownloaders.Remove)
			controller.Run()
		}()
	}

	if mainContext.Src != nil {
		wg.Add(1)

		go func() {
			defer wg.Done()

			downloader := NewLocalDownloader(mainContext.ChunkManager, mainContext.Src)
			defer downloader.Fin()

			execDownloader(mainContext, downloader, *flagLocalNumConcurrent)
		}()
	}

	// If LastUpdatedAt is not updated within the duration specified by flagTransferIdleTimeout, a timeout occurs.
	// Incomplete file transfer. Program will terminate with exit code 1.
	if *flagTransferIdleTimeout > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ticker := time.NewTicker(constants.PeerWaitDuration)
			defer ticker.Stop()

			mainContext.LastUpdatedAt.Store(t0)

			for {
				select {
				case <-mainContext.DownloadCtx.Done():
					return
				case <-ticker.C:
					if time.Since(mainContext.LastUpdatedAt.Load()) >= *flagTransferIdleTimeout {
						utils.ErrorPrintf("Transfer idle timeout %v exceeded.", *flagTransferIdleTimeout)
						mainContext.ExitCode.Store(1)
						mainContext.DownloadCancel()
						return
					}
				}
			}
		}()
	}

	wg.Wait()

	err = transfer.CompleteTransfer(mainContext.ChunkManager, mainContext.Dst)
	if err != nil {
		return err
	}

	t1 := time.Now()
	elapsed := t1.Sub(t0)

	mainContext.Statistics.SetElapsed(elapsed)

	if *flagVerifyOnComplete {
		utils.DebugPrintf("Verification of the destination file has started.")

		if !transfer.VerifyTransfer(mainContext.ChunkManager, mainContext.Dst) {
			mainContext.ExitCode.Store(1)
		}

		utils.DebugPrintf("Verification of the destination file has ended.")
	}

	mainContext.Command.RunOnComplete(mainContext.ChunkManager.GetFileCount(), mainContext.Statistics.GetBytesReceivedTotal(), elapsed.String())

	utils.DebugPrintf("All download workers finished.")
	return nil
}

func execDownloader(mainContext *MainContext, downloader Downloader, numPeerConcurrent int) {
	queue := downloader.GetQueue()
	nameDownloader := downloader.Name()

	// No chunks to download.
	if mainContext.ChunkManager.GetChunkCount() == 0 {
		mainContext.DownloadCancel()
		mainContext.Completed.Store(true)
		utils.DebugPrintf("[%s] download job done.", nameDownloader)
	}

	var numCompleted atomic.Int64
	CompleteOne := func() {
		numCompleted.Add(1)

		// Download of all chunks is complete.
		if numCompleted.Load() == int64(mainContext.ChunkManager.GetChunkCount()) {
			mainContext.DownloadCancel()
			mainContext.Completed.Store(true)
			utils.DebugPrintf("[%s] download job done.", nameDownloader)
		}
	}

	var wgWorker sync.WaitGroup
	defer wgWorker.Wait()

	// Create a channel to pass the chunks to be transmitted to the downloadWorker goroutine.
	chanRequest := make(chan int)
	defer close(chanRequest)

	var bytesReceivedTotal atomic.Int64
	var chunksReceivedTotal atomic.Int64
	var durationTotal utils.AtomicDuration

	var downloadWorkerReadyCount atomic.Int32
	downloadWorker := func() {
		defer wgWorker.Done()
		for {
			downloadWorkerReadyCount.Add(1)
			chunkIndex, ok := <-chanRequest
			downloadWorkerReadyCount.Add(-1)
			if !ok {
				break
			}

			t0 := time.Now()
			length, err := func() (int64, error) {
				checksum, reader, err := downloader.RequestDownload(chunkIndex, *flagVerifyOnComplete)
				if err != nil {
					return 0, err
				}
				defer reader.Close()

				length, err := mainContext.ChunkManager.WriteChunk(mainContext.Dst, chunkIndex, reader, checksum, mainContext.Command.RunOnEachFileComplete)
				if err != nil {
					return 0, err
				}

				if *flagVerifyOnComplete {
					mainContext.ChunkManager.SetChunkChecksum(chunkIndex, checksum)
				}

				return length, nil
			}()
			t1 := time.Now()

			if err == nil {
				mainContext.ChunkManager.SetChunkStatusDone(chunkIndex, t1)
				utils.DebugPrintf("[%s] downloaded %d bytes of chunk:%v, elapsed:%v", nameDownloader, length, chunkIndex, t1.Sub(t0))
				CompleteOne()

				mainContext.LastUpdatedAt.Store(t1)

				chunksReceivedTotal.Add(1)
				bytesReceivedTotal.Add(length)
				durationTotal.Add(t1.Sub(t0))
			} else {
				// If the transmission result is an error, retry the chunkIndex.
				mainContext.ChunkManager.SetChunkStatusPending(chunkIndex)
				queue.PushBack(chunkIndex)

				utils.ErrorPrintf("[%s] %v Chunk download failed.: %s", nameDownloader, chunkIndex, err.Error())

				time.Sleep(constants.PeerWaitDuration)
			}
		}
	}
	wgWorker.Add(numPeerConcurrent)
	for i := 0; i < numPeerConcurrent; i++ {
		go downloadWorker()
	}

	for {
		if !downloader.IsValid() {
			break
		}

		if mainContext.DownloadCtx.Err() != nil {
			break
		}

		if downloadWorkerReadyCount.Load() == 0 {
			time.Sleep(1 * time.Millisecond)
			continue
		}

		chunkIndex, ok := queue.Pop()
		if ok {
			switch mainContext.ChunkManager.GetChunkStatus(chunkIndex) {
			case types.ChunkStatusPending:
				// Attempt to change the status from Pending to Downloading if the chunk status is Pending.
				if mainContext.ChunkManager.UpdateChunkStatus(chunkIndex, types.ChunkStatusPending, types.ChunkStatusDownloading) {
					// Chunk to be transferred
					chanRequest <- chunkIndex
				} else {
					// Status changed by another downloader. Retrying.
					queue.PushBack(chunkIndex)
				}

			case types.ChunkStatusDownloading:
				// Chunk being transferred by another downloader. Holding off.
				queue.PushBack(chunkIndex)

			case types.ChunkStatusDone:
				// Chunk already transferred by another downloader. Marking as completed in this downloader as well.
				CompleteOne()
			}
		} else {
			// Queue is empty, waiting.
			time.Sleep(1 * time.Millisecond)
		}
	}

	mainContext.Statistics.AddDownloader(downloader.Name(), bytesReceivedTotal.Load(), chunksReceivedTotal.Load(), durationTotal.Load())
}
