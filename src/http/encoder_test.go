// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package http

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestGetPreferredEncodingType(t *testing.T) {
	tests := []struct {
		name   string
		header []string
		want   string
	}{
		{
			name:   "empty header",
			header: []string{},
			want:   EncodingTypeNone,
		},
		{
			name:   "single zstd",
			header: []string{"zstd"},
			want:   EncodingTypeZstd,
		},
		{
			name:   "single none",
			header: []string{"none"},
			want:   EncodingTypeNone,
		},
		{
			name:   "zstd preferred over none",
			header: []string{"none,zstd"},
			want:   EncodingTypeZstd,
		},
		{
			name:   "multiple entries with spaces",
			header: []string{"none, zstd"},
			want:   EncodingTypeZstd,
		},
		{
			name:   "multiple header values",
			header: []string{"none", "zstd"},
			want:   EncodingTypeZstd,
		},
		{
			name:   "unknown encoding ignored",
			header: []string{"gzip", "unknown"},
			want:   EncodingTypeNone,
		},
		{
			name:   "unknown with zstd",
			header: []string{"gzip", "zstd", "unknown"},
			want:   EncodingTypeZstd,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetPreferredEncodingType(tt.header)
			if got != tt.want {
				t.Errorf("GetPreferredEncodingType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsAvailableEncoding(t *testing.T) {
	tests := []struct {
		name  string
		input *string
		want  bool
	}{
		{
			name:  "nil pointer",
			input: nil,
			want:  false,
		},
		{
			name:  "empty string becomes none",
			input: stringPtr(""),
			want:  true,
		},
		{
			name:  "none",
			input: stringPtr("none"),
			want:  true,
		},
		{
			name:  "zstd",
			input: stringPtr("zstd"),
			want:  true,
		},
		{
			name:  "unknown encoding",
			input: stringPtr("gzip"),
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input *string
			if tt.input != nil {
				s := *tt.input
				input = &s
			}
			got := IsAvailableEncoding(input)
			if got != tt.want {
				t.Errorf("IsAvailableEncoding() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("empty string is changed to none", func(t *testing.T) {
		input := stringPtr("")
		IsAvailableEncoding(input)
		if *input != EncodingTypeNone {
			t.Errorf("expected %q, got %q", EncodingTypeNone, *input)
		}
	})
}

func TestNopWriteCloser(t *testing.T) {
	buf := &bytes.Buffer{}
	wc := NopWriteCloser(buf)

	// Write data
	testData := []byte("test data")
	n, err := wc.Write(testData)
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(testData), n)
	}

	// Close should not error
	if err := wc.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Verify data was written
	if buf.String() != string(testData) {
		t.Errorf("Expected buffer to contain '%s', got '%s'", testData, buf.String())
	}
}

func TestNewEncodingReadCloser(t *testing.T) {
	t.Run("none encoding passthrough", func(t *testing.T) {
		testData := []byte("hello world")
		rc := io.NopCloser(bytes.NewReader(testData))
		erc := NewEncodingReadCloser(rc, EncodingTypeNone)

		data, err := io.ReadAll(erc)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}
		if !bytes.Equal(data, testData) {
			t.Errorf("Expected data '%s', got '%s'", testData, data)
		}

		if err := erc.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	t.Run("zstd decoding", func(t *testing.T) {
		testData := []byte("hello world, this is a test of zstd compression")

		// Compress the data first
		compressedReader, _, err := NewBufferEncodingReader(testData, EncodingTypeZstd)
		if err != nil {
			t.Fatalf("NewBufferEncodingReader failed: %v", err)
		}
		compressedData, err := io.ReadAll(compressedReader)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}

		// Now decompress it
		rc := io.NopCloser(bytes.NewReader(compressedData))
		erc := NewEncodingReadCloser(rc, EncodingTypeZstd)

		decompressed, err := io.ReadAll(erc)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}
		if !bytes.Equal(decompressed, testData) {
			t.Errorf("Expected data '%s', got '%s'", testData, decompressed)
		}

		if err := erc.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})
}

func TestNewBufferEncodingReader(t *testing.T) {
	t.Run("none encoding", func(t *testing.T) {
		testData := []byte("test data")
		reader, size, err := NewBufferEncodingReader(testData, EncodingTypeNone)
		if err != nil {
			t.Fatalf("NewBufferEncodingReader failed: %v", err)
		}
		if size != int64(len(testData)) {
			t.Errorf("Expected size %d, got %d", len(testData), size)
		}

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}
		if !bytes.Equal(data, testData) {
			t.Errorf("Expected data '%s', got '%s'", testData, data)
		}
	})

	t.Run("zstd compression", func(t *testing.T) {
		testData := []byte(strings.Repeat("test data that should compress well ", 100))
		reader, size, err := NewBufferEncodingReader(testData, EncodingTypeZstd)
		if err != nil {
			t.Fatalf("NewBufferEncodingReader failed: %v", err)
		}

		// Compressed size should be smaller than original
		if size >= int64(len(testData)) {
			t.Logf("Warning: compressed size (%d) >= original size (%d)", size, len(testData))
		}

		compressedData, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}
		if int64(len(compressedData)) != size {
			t.Errorf("Size mismatch: reported %d, actual %d", size, len(compressedData))
		}
	})
}

func TestNewEncodingWriteCloser(t *testing.T) {
	t.Run("none encoding", func(t *testing.T) {
		buf := &bytes.Buffer{}
		wc := NewEncodingWriteCloser(buf, EncodingTypeNone)

		testData := []byte("test data")
		n, err := wc.Write(testData)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		if n != len(testData) {
			t.Errorf("Expected to write %d bytes, wrote %d", len(testData), n)
		}

		if err := wc.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}

		if !bytes.Equal(buf.Bytes(), testData) {
			t.Errorf("Expected buffer to contain '%s', got '%s'", testData, buf.Bytes())
		}
	})

	t.Run("zstd compression", func(t *testing.T) {
		buf := &bytes.Buffer{}
		wc := NewEncodingWriteCloser(buf, EncodingTypeZstd)

		testData := []byte(strings.Repeat("compressible data ", 100))
		n, err := wc.Write(testData)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		if n != len(testData) {
			t.Errorf("Expected to write %d bytes, wrote %d", len(testData), n)
		}

		if err := wc.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}

		// Compressed data should be different from original
		if bytes.Equal(buf.Bytes(), testData) {
			t.Error("Compressed data should differ from original")
		}

		// Verify we can decompress it
		rc := NewEncodingReadCloser(io.NopCloser(bytes.NewReader(buf.Bytes())), EncodingTypeZstd)
		decompressed, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("Decompression failed: %v", err)
		}
		if !bytes.Equal(decompressed, testData) {
			t.Error("Decompressed data doesn't match original")
		}
	})
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
