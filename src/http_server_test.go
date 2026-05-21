// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSetEncodingType(t *testing.T) {
	tests := []struct {
		name           string
		acceptEncoding []string
		wantEncoding   string
		wantHeader     string
	}{
		{
			name:           "no encoding",
			acceptEncoding: nil,
			wantEncoding:   "none",
			wantHeader:     "",
		},
		{
			name:           "zstd encoding",
			acceptEncoding: []string{"zstd"},
			wantEncoding:   "zstd",
			wantHeader:     "zstd",
		},
		{
			name:           "multiple encodings prefer zstd",
			acceptEncoding: []string{"gzip, zstd, br"},
			wantEncoding:   "zstd",
			wantHeader:     "zstd",
		},
		{
			name:           "unsupported encoding",
			acceptEncoding: []string{"br"},
			wantEncoding:   "none",
			wantHeader:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.acceptEncoding != nil {
				req.Header["Accept-Encoding"] = tt.acceptEncoding
			}
			w := httptest.NewRecorder()

			got := setEncodingType(w, req)
			if got != tt.wantEncoding {
				t.Errorf("setEncodingType() = %v, want %v", got, tt.wantEncoding)
			}

			gotHeader := w.Header().Get("Content-Encoding")
			if gotHeader != tt.wantHeader {
				t.Errorf("Content-Encoding header = %v, want %v", gotHeader, tt.wantHeader)
			}
		})
	}
}

func TestGetWantDigest(t *testing.T) {
	tests := []struct {
		name       string
		wantDigest []string
		want       bool
	}{
		{
			name:       "no want-digest header",
			wantDigest: nil,
			want:       false,
		},
		{
			name:       "xxh3 digest",
			wantDigest: []string{"xxh3"},
			want:       true,
		},
		{
			name:       "xxh3 with spaces",
			wantDigest: []string{" xxh3 "},
			want:       true,
		},
		{
			name:       "other digest",
			wantDigest: []string{"sha256"},
			want:       false,
		},
		{
			name:       "empty header",
			wantDigest: []string{""},
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.wantDigest != nil {
				req.Header["Want-Digest"] = tt.wantDigest
			}

			got := getWantDigest(req)
			if got != tt.want {
				t.Errorf("getWantDigest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetDigest(t *testing.T) {
	w := httptest.NewRecorder()
	checksum := uint64(12345678901234)

	setDigest(w, checksum)

	got := w.Header().Get("Digest")
	want := "xxh3=12345678901234"
	if got != want {
		t.Errorf("setDigest() header = %v, want %v", got, want)
	}
}

func TestParseDigest(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		want    uint64
		wantErr bool
	}{
		{
			name:    "valid digest",
			header:  "xxh3=12345678901234",
			want:    12345678901234,
			wantErr: false,
		},
		{
			name:    "invalid format - no equals",
			header:  "xxh312345678901234",
			want:    0,
			wantErr: true,
		},
		{
			name:    "invalid format - too many parts",
			header:  "xxh3=123=456",
			want:    0,
			wantErr: true,
		},
		{
			name:    "unsupported digest type",
			header:  "sha256=abc123",
			want:    0,
			wantErr: true,
		},
		{
			name:    "invalid number",
			header:  "xxh3=notanumber",
			want:    0,
			wantErr: true,
		},
		{
			name:    "empty header",
			header:  "",
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := http.Header{}
			header.Set("Digest", tt.header)

			got, err := ParseDigest(header)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDigest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseDigest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetOnlyUpdatedSince(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		want    time.Time
		wantErr bool
	}{
		{
			name:    "no parameter",
			query:   "",
			want:    time.Time{},
			wantErr: false,
		},
		{
			name:    "empty parameter",
			query:   "only_updated_since=",
			want:    time.Time{},
			wantErr: false,
		},
		{
			name:    "valid RFC3339Nano",
			query:   "only_updated_since=2024-01-15T10:30:00.123456789Z",
			want:    time.Date(2024, 1, 15, 10, 30, 0, 123456789, time.UTC),
			wantErr: false,
		},
		{
			name:    "invalid timestamp",
			query:   "only_updated_since=invalid",
			want:    time.Time{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/"
			if tt.query != "" {
				url = "/?" + tt.query
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)

			got, err := getOnlyUpdatedSince(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("getOnlyUpdatedSince() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !got.Equal(tt.want) {
				t.Errorf("getOnlyUpdatedSince() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTextResponse(t *testing.T) {
	stat := NewStatistics(false)
	w := httptest.NewRecorder()

	err := textResponse(stat, w, "test content", http.StatusOK)
	if err != nil {
		t.Errorf("textResponse() error = %v", err)
	}

	if w.Code != http.StatusOK {
		t.Errorf("status code = %v, want %v", w.Code, http.StatusOK)
	}

	if w.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %v, want text/plain; charset=utf-8", w.Header().Get("Content-Type"))
	}

	if w.Header().Get("Content-Length") != "12" {
		t.Errorf("Content-Length = %v, want 12", w.Header().Get("Content-Length"))
	}

	if w.Body.String() != "test content" {
		t.Errorf("body = %v, want test content", w.Body.String())
	}

	// Verify statistics were updated
	if stat.BytesTransmittedTotal.Load() != 12 {
		t.Errorf("BytesTransmittedTotal = %v, want 12", stat.BytesTransmittedTotal.Load())
	}
}

func TestErrorResponse(t *testing.T) {
	stat := NewStatistics(false)
	w := httptest.NewRecorder()

	errorResponse(stat, w, http.StatusBadRequest, http.ErrBodyNotAllowed)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code = %v, want %v", w.Code, http.StatusBadRequest)
	}
}

func TestAvailableListResponse(t *testing.T) {
	tests := []struct {
		name           string
		availableList  AvailableList
		acceptEncoding string
		wantStatus     int
	}{
		{
			name: "simple available list without encoding",
			availableList: AvailableList{
				Timestamp:              time.Now(),
				IsCompleted:            false,
				TransferChunkIndexList: []int{0, 1, 2},
			},
			acceptEncoding: "",
			wantStatus:     http.StatusOK,
		},
		{
			name: "completed available list",
			availableList: AvailableList{
				Timestamp:   time.Now(),
				IsCompleted: true,
			},
			acceptEncoding: "",
			wantStatus:     http.StatusOK,
		},
		{
			name: "available list with zstd encoding",
			availableList: AvailableList{
				Timestamp:              time.Now(),
				IsCompleted:            false,
				TransferChunkIndexList: []int{0, 1, 2, 3, 4},
			},
			acceptEncoding: "zstd",
			wantStatus:     http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stat := NewStatistics(false)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/chunk", nil)
			if tt.acceptEncoding != "" {
				req.Header.Set("Accept-Encoding", tt.acceptEncoding)
			}

			availableListResponse(stat, w, req, tt.availableList)

			if w.Code != tt.wantStatus {
				t.Errorf("status code = %v, want %v", w.Code, tt.wantStatus)
			}

			if w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %v, want application/json; charset=utf-8", w.Header().Get("Content-Type"))
			}

			if w.Body.Len() == 0 {
				t.Error("Expected non-empty body")
			}

			if stat.BytesTransmittedTotal.Load() == 0 {
				t.Error("Expected bytes transmitted to be updated")
			}

			// Verify actual JSON content by decoding it
			body := w.Body.Bytes()
			// If zstd encoded, decode first
			if tt.acceptEncoding == "zstd" {
				// Skip JSON verification for compressed response
				// Just verify Content-Encoding header is set
				if w.Header().Get("Content-Encoding") != "zstd" {
					t.Errorf("Expected Content-Encoding 'zstd', got '%s'", w.Header().Get("Content-Encoding"))
				}
			} else {
				var decoded AvailableList
				if err := json.Unmarshal(body, &decoded); err != nil {
					t.Fatalf("Failed to unmarshal response body: %v", err)
				}

				// Verify IsCompleted matches
				if decoded.IsCompleted != tt.availableList.IsCompleted {
					t.Errorf("IsCompleted = %v, want %v", decoded.IsCompleted, tt.availableList.IsCompleted)
				}

				// Verify TransferChunkIndexList matches
				if len(decoded.TransferChunkIndexList) != len(tt.availableList.TransferChunkIndexList) {
					t.Errorf("TransferChunkIndexList length = %d, want %d",
						len(decoded.TransferChunkIndexList), len(tt.availableList.TransferChunkIndexList))
				} else {
					for i, idx := range decoded.TransferChunkIndexList {
						if idx != tt.availableList.TransferChunkIndexList[i] {
							t.Errorf("TransferChunkIndexList[%d] = %d, want %d",
								i, idx, tt.availableList.TransferChunkIndexList[i])
						}
					}
				}
			}
		})
	}
}

func TestCloseServer(t *testing.T) {
	// Create a simple test server
	server := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: http.NewServeMux(),
	}

	// CloseServer should not error on unstarted server
	err := CloseServer(server)
	if err != nil {
		t.Errorf("CloseServer() error = %v", err)
	}
}
