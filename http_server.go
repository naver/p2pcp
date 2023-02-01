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
	"os"
	"strconv"
	"strings"
	"time"
)

func textResponse(w http.ResponseWriter, content string, status int) error {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
	w.WriteHeader(status)

	written, err := w.Write([]byte(content))
	mainContext.BytesTransmittedTotal.Add(int64(written))

	return err
}

func errorResponse(w http.ResponseWriter, status int, err error) {
	textResponse(w, err.Error(), status)
}

func setEncodingType(w http.ResponseWriter, r *http.Request) string {
	encodingType := GetPreferredEncodingType(r.Header["Accept-Encoding"])
	if encodingType != EncodingTypeNone {
		w.Header().Set("Content-Encoding", encodingType)
	}
	return encodingType
}

func CloseServer(mainContext *MainContext, server *http.Server) error {
	shutdownContext, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()
	return server.Shutdown(shutdownContext)
}

func CreateHttpServer(mainContext *MainContext) *http.Server {

	mux := http.NewServeMux()

	/* 진행사항 모니터링 위한 핸들러 */
	mux.HandleFunc("/completed", func(w http.ResponseWriter, r *http.Request) {
		var status int
		var content string
		if !mainContext.Completed.Load() {
			status = http.StatusNotFound
			content = "Downloading"
		} else {
			status = http.StatusOK
			content = "OK"
		}

		err := textResponse(w, content, status)
		if err != nil {
			ErrorPrintf("Write failed.:%v", err)
			return
		}
	})

	/* 연결된 peer가 자기자신인지 확인하기 위한 핸들러 */
	mux.HandleFunc("/uuid", func(w http.ResponseWriter, r *http.Request) {
		err := textResponse(w, mainContext.UUID, http.StatusOK)
		if err != nil {
			ErrorPrintf("Write failed.:%v", err)
			return
		}
	})

	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		err := textResponse(w, getVersion(), http.StatusOK)
		if err != nil {
			ErrorPrintf("Write failed.:%v", err)
			return
		}
	})

	mux.HandleFunc("/manifest", func(w http.ResponseWriter, r *http.Request) {
		encodingType := setEncodingType(w, r)
		reader, length, err := NewBufferEncodingReader(mainContext.Manifest.JsonData, encodingType)
		if err != nil {
			ErrorPrintf("NewBufferEncodingReader failed.:%v", err)
			errorResponse(w, http.StatusInternalServerError, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", length))

		written, err := io.Copy(w, reader)
		mainContext.BytesTransmittedTotal.Add(written)
		if err != nil {
			ErrorPrintf("io.Copy failed.:%v", err)
			errorResponse(w, http.StatusInternalServerError, err)
			return
		}
	})

	mux.HandleFunc("/manifest/checksum", func(w http.ResponseWriter, r *http.Request) {
		err := textResponse(w, mainContext.Manifest.ChunkChecksum, http.StatusOK)
		if err != nil {
			ErrorPrintf("Write failed.:%v", err)
			return
		}
	})

	mux.HandleFunc("/chunk", func(w http.ResponseWriter, r *http.Request) {
		onlyUpdatedSinceArray := r.URL.Query()["only_updated_since"]
		var onlyUpdatedSinceStr string
		if len(onlyUpdatedSinceArray) > 0 {
			onlyUpdatedSinceStr = onlyUpdatedSinceArray[0]
		}

		now := time.Now().Format(time.RFC3339Nano)
		resultObject := AvailableChunkList{
			Timestamp: now,
		}

		if mainContext.Completed.Load() {
			resultObject.IsCompleted = true
		} else {
			var onlyUpdatedSince time.Time
			if onlyUpdatedSinceStr != "" {
				var err error
				onlyUpdatedSince, err = time.Parse(time.RFC3339Nano, onlyUpdatedSinceStr)
				if err != nil {
					ErrorPrintf("invalid timestamp.:only_updated_since=%v, %v", onlyUpdatedSinceStr, err.Error())
					errorResponse(w, http.StatusBadRequest, err)
					return
				}
			}

			for index := range mainContext.TransferChunks {
				if mainContext.TransferChunks[index].Status.Load() == ChunkStatusDone &&
					onlyUpdatedSince.After(mainContext.TransferChunks[index].LastUpdatedAt.Load()) {
					resultObject.TransferChunkIndexList = append(resultObject.TransferChunkIndexList, index)
				}
			}
		}

		json, err := json.Marshal(resultObject)
		if err != nil {
			ErrorPrintf("JSON Marshal failed.:%v", err)
			errorResponse(w, http.StatusInternalServerError, err)
			return
		}

		encodingType := setEncodingType(w, r)
		reader, length, err := NewBufferEncodingReader(json, encodingType)
		if err != nil {
			ErrorPrintf("NewBufferEncodingReader failed.:%v", err)
			errorResponse(w, http.StatusInternalServerError, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", length))

		written, err := io.Copy(w, reader)
		mainContext.BytesTransmittedTotal.Add(written)
		if err != nil {
			ErrorPrintf("io.Copy failed.:%v", err)
			errorResponse(w, http.StatusInternalServerError, err)
			return
		}
	})

	const servePointFiles = "/chunk/"
	var htdocs string
	var GetChunkStatus func(int) int32
	if mainContext.ServeOnly {
		htdocs = mainContext.DirSrc
		GetChunkStatus = func(int) int32 { return ChunkStatusDone }
	} else {
		htdocs = mainContext.DirDst
		GetChunkStatus = func(transferChunkIndex int) int32 {
			return mainContext.TransferChunks[transferChunkIndex].Status.Load()
		}
	}
	mux.HandleFunc(servePointFiles, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, servePointFiles)
		transferChunkIndex, err := strconv.Atoi(path)
		if err != nil {
			ErrorPrintf("strconv.Atoi failed:%v. got (%s)", err, r.URL.Path)
			errorResponse(w, http.StatusForbidden, errors.New("chunk index is not an integer"))
			return
		}

		if transferChunkIndex < 0 || transferChunkIndex >= mainContext.Manifest.TransferChunkCount {
			ErrorPrintf("chunk index is out of range. should be in range 0 ~ %d. got (%d)", mainContext.Manifest.TransferChunkCount, transferChunkIndex)
			errorResponse(w, http.StatusForbidden, errors.New("chunk index out of range"))
			return
		}

		status := GetChunkStatus(transferChunkIndex)
		if status != ChunkStatusDone {
			ErrorPrintf("chunk not ready: chunk:%v, %v", transferChunkIndex, status)
			errorResponse(w, http.StatusForbidden, errors.New("chunk not ready"))
			return
		}

		encodingType := setEncodingType(w, r)
		writer := NewEncodingWriteCloser(w, encodingType)
		defer writer.Close()

		w.Header().Set("Content-Type", "application/octet-stream")
		if encodingType == EncodingTypeNone {
			length := mainContext.TransferChunks[transferChunkIndex].Size()
			w.Header().Set("Content-Length", fmt.Sprintf("%d", length))
		}

		for _, fileChunk := range mainContext.TransferChunks[transferChunkIndex].FileChunks {
			file := &mainContext.Manifest.Files[fileChunk.FileIndex]

			f, err := os.Open(htdocs + file.Name)
			if err != nil {
				ErrorPrintf("Open failed:%v, file:%v", err, htdocs+file.Name)
				errorResponse(w, http.StatusInternalServerError, err)
				return
			}
			defer f.Close()

			_, err = f.Seek(fileChunk.FromOffset, io.SeekStart)
			if err != nil {
				ErrorPrintf("Seek failed:%v, file:%v", err, htdocs+file.Name)
				errorResponse(w, http.StatusInternalServerError, err)
				return
			}

			written, err := io.CopyN(writer, f, fileChunk.ToOffset-fileChunk.FromOffset)
			if err != nil {
				ErrorPrintf("Copy failed:%v, file:%v", err, htdocs+file.Name)
				errorResponse(w, http.StatusInternalServerError, err)
				return
			}
			mainContext.BytesTransmittedTotal.Add(written)
		}
	})

	/*
		// net/http/pprof import 필요
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	*/

	return &http.Server{
		Addr:         *flagListenAddr,
		Handler:      mux,
		ReadTimeout:  1 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
}
