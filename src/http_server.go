// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/naver/p2pcp/constants"
	p2pcpHttp "github.com/naver/p2pcp/http"
	"github.com/naver/p2pcp/storage"
	"github.com/naver/p2pcp/types"
	"github.com/naver/p2pcp/utils"
	"github.com/naver/p2pcp/version"
)

func textResponse(stat *Statistics, w http.ResponseWriter, content string, status int) error {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
	w.WriteHeader(status)

	written, err := w.Write([]byte(content))
	stat.AddBytesTransmittedTotal(int64(written))

	return err
}

func errorResponse(stat *Statistics, w http.ResponseWriter, status int, err error) {
	textResponse(stat, w, err.Error(), status)
}

func setEncodingType(w http.ResponseWriter, r *http.Request) string {
	encodingType := p2pcpHttp.GetPreferredEncodingType(r.Header["Accept-Encoding"])
	if encodingType != p2pcpHttp.EncodingTypeNone {
		w.Header().Set("Content-Encoding", encodingType)
	}
	return encodingType
}

func getWantDigest(r *http.Request) bool {
	headerArray := r.Header["Want-Digest"]
	if len(headerArray) == 0 {
		return false
	}

	return strings.TrimSpace(headerArray[0]) == constants.DefaultDigest
}

func setDigest(w http.ResponseWriter, checksum uint64) {
	w.Header().Set("Digest", fmt.Sprintf("%s=%d", constants.DefaultDigest, checksum))
}

func ParseDigest(header http.Header) (uint64, error) {
	array := strings.Split(header.Get("Digest"), "=")
	if len(array) != 2 {
		return 0, fmt.Errorf("failed to get digest (%v)", array)
	}
	if array[0] != "xxh3" {
		return 0, fmt.Errorf("unsupported digest (%v)", array)
	}
	checksum, err := strconv.ParseUint(array[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse digest (%s)", array[1])
	}
	return checksum, nil
}

func getOnlyUpdatedSince(r *http.Request) (time.Time, error) {
	onlyUpdatedSinceArray := r.URL.Query()["only_updated_since"]
	if len(onlyUpdatedSinceArray) > 0 && onlyUpdatedSinceArray[0] != "" {
		onlyUpdatedSince, err := time.Parse(time.RFC3339Nano, onlyUpdatedSinceArray[0])
		if err != nil {
			utils.ErrorPrintf("Invalid timestamp.: only_updated_since=%s (%s)", onlyUpdatedSinceArray[0], err.Error())
		}
		return onlyUpdatedSince, err
	}
	return time.Time{}, nil
}

func availableListResponse(stat *Statistics, w http.ResponseWriter, r *http.Request, availableList interface{}) {
	jsonData, err := json.Marshal(availableList)
	if err != nil {
		utils.ErrorPrintf("Failed to marshal JSON.: %s", err.Error())
		errorResponse(stat, w, http.StatusInternalServerError, err)
		return
	}

	encodingType := setEncodingType(w, r)
	reader, length, err := p2pcpHttp.NewBufferEncodingReader(jsonData, encodingType)
	if err != nil {
		utils.ErrorPrintf("Failed to create a reader for sending the encoded response.: %s", err.Error())
		errorResponse(stat, w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", length))

	written, err := io.Copy(w, reader)
	stat.AddBytesTransmittedTotal(written)
	if err != nil {
		utils.ErrorPrintf("Failed to send the response.: %s", err.Error())
		errorResponse(stat, w, http.StatusInternalServerError, err)
		return
	}
}

func CloseServer(server *http.Server) error {
	shutdownContext, cancel := context.WithTimeout(context.Background(), constants.ShutdownTimeout)
	defer cancel()
	return server.Shutdown(shutdownContext)
}

func CreateHttpServer(mainContext *MainContext) *http.Server {

	mux := http.NewServeMux()

	muxWrapping := func(pattern string, handler func(http.ResponseWriter, *http.Request)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			handler(w, r)
			mainContext.LastSentAt.Store(time.Now())
		})
	}

	// Handler for monitoring progress.
	muxWrapping("/completed", func(w http.ResponseWriter, r *http.Request) {
		var status int
		var content string
		if !mainContext.Completed.Load() {
			status = http.StatusNotFound
			content = "Downloading"
		} else {
			status = http.StatusOK
			content = "OK"
		}

		err := textResponse(mainContext.Statistics, w, content, status)
		if err != nil {
			utils.ErrorPrintf("Failed to send the response.: %s", err.Error())
			return
		}
	})

	// Handler for checking if the peer is oneself.
	muxWrapping("/uuid", func(w http.ResponseWriter, r *http.Request) {
		err := textResponse(mainContext.Statistics, w, mainContext.UUID, http.StatusOK)
		if err != nil {
			utils.ErrorPrintf("Failed to send the response.: %s", err.Error())
			return
		}
	})

	// Handler for checking the version.
	muxWrapping("/version", func(w http.ResponseWriter, r *http.Request) {
		err := textResponse(mainContext.Statistics, w, version.GetVersion(), http.StatusOK)
		if err != nil {
			utils.ErrorPrintf("Failed to send the response.: %s", err.Error())
			return
		}
	})

	// Handler for retrieving the manifest to prepare the transfer list.
	muxWrapping("/manifest", func(w http.ResponseWriter, r *http.Request) {
		encodingType := setEncodingType(w, r)
		reader, length, err := p2pcpHttp.NewBufferEncodingReader(mainContext.ChunkManager.GetManifestJson(), encodingType)
		if err != nil {
			utils.ErrorPrintf("Failed to create a reader for sending the encoded response.: %s", err.Error())
			errorResponse(mainContext.Statistics, w, http.StatusInternalServerError, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", length))

		written, err := io.Copy(w, reader)
		mainContext.Statistics.AddBytesTransmittedTotal(written)
		if err != nil {
			utils.ErrorPrintf("Failed to send the response.: %s", err.Error())
			errorResponse(mainContext.Statistics, w, http.StatusInternalServerError, err)
			return
		}
	})

	// Handler for retrieving the manifest checksum to verify the integrity of the transfer list.
	muxWrapping("/manifest/checksum", func(w http.ResponseWriter, r *http.Request) {
		err := textResponse(mainContext.Statistics, w, strconv.FormatUint(mainContext.ChunkManager.GetManifestChecksum(), 10), http.StatusOK)
		if err != nil {
			utils.ErrorPrintf("Failed to send the response.: %s", err.Error())
			return
		}
	})

	// Handler for updating the AvailableList to generate the transfer chunk status list.
	muxWrapping("/chunk", func(w http.ResponseWriter, r *http.Request) {
		availableList := NewAvailableList()

		if mainContext.Completed.Load() {
			availableList.IsCompleted = true
		} else {
			onlyUpdatedSince, err := getOnlyUpdatedSince(r)
			if err != nil {
				errorResponse(mainContext.Statistics, w, http.StatusBadRequest, err)
				return
			}

			availableList.TransferChunkIndexList = mainContext.ChunkManager.GetStatusDoneChunks(onlyUpdatedSince)
		}

		availableListResponse(mainContext.Statistics, w, r, availableList)
	})

	const servePointFile = "/chunk/"
	var storage storage.Storage
	var GetChunkStatus func(int) types.ChunkStatus
	if mainContext.ServeOnly {
		storage = mainContext.Src
		GetChunkStatus = func(int) types.ChunkStatus { return types.ChunkStatusDone }
	} else {
		storage = mainContext.Dst
		GetChunkStatus = func(chunkIndex int) types.ChunkStatus {
			if mainContext.Completed.Load() {
				return types.ChunkStatusDone
			}
			return mainContext.ChunkManager.GetChunkStatus(chunkIndex)
		}
	}

	// Handler for fetching a chunk.
	muxWrapping(servePointFile, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, servePointFile)
		chunkIndex, err := strconv.Atoi(path)
		if err != nil {
			utils.ErrorPrintf("strconv.Atoi failed:%v. got (%s)", err, r.URL.Path)
			errorResponse(mainContext.Statistics, w, http.StatusForbidden, errors.New("chunk index is not an integer"))
			return
		}

		if chunkIndex < 0 || chunkIndex >= mainContext.ChunkManager.GetChunkCount() {
			utils.ErrorPrintf("chunk index is out of range. should be in range 0 ~ %d. got (%d)", mainContext.ChunkManager.GetChunkCount(), chunkIndex)
			errorResponse(mainContext.Statistics, w, http.StatusForbidden, errors.New("chunk index out of range"))
			return
		}

		status := GetChunkStatus(chunkIndex)
		if status != types.ChunkStatusDone {
			utils.ErrorPrintf("chunk not ready: chunk:%v, %v", chunkIndex, status)
			errorResponse(mainContext.Statistics, w, http.StatusForbidden, errors.New("chunk not ready"))
			return
		}

		encodingType := setEncodingType(w, r)
		writer := p2pcpHttp.NewEncodingWriteCloser(w, encodingType)
		defer writer.Close()

		w.Header().Set("Content-Type", "application/octet-stream")
		if encodingType == p2pcpHttp.EncodingTypeNone {
			length := mainContext.ChunkManager.GetChunkSize(chunkIndex)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", length))
		}

		if getWantDigest(r) {
			checksum, err := mainContext.ChunkManager.ComputeChunkChecksum(storage, chunkIndex)
			if err != nil {
				utils.ErrorPrintf("Failed to get chunk checksum.: %s", err.Error())
				errorResponse(mainContext.Statistics, w, http.StatusInternalServerError, err)
				return
			}
			setDigest(w, checksum)
		}

		readCloser := mainContext.ChunkManager.GetChunkReader(storage, chunkIndex)
		defer readCloser.Close()

		written, err := io.Copy(writer, readCloser)
		mainContext.Statistics.AddBytesTransmittedTotal(written)
		if err != nil {
			utils.ErrorPrintf("Failed to send the response.: %s", err.Error())
			return
		}
	})

	server := &http.Server{
		Addr:         *flagListenAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	return server
}
