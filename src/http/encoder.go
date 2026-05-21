// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package http

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/valyala/gozstd"
)

const (
	EncodingTypeNone = "none"
	EncodingTypeZstd = "zstd"
)

// Lower numbers indicate higher priority for encoding.
var EncodingPriorMap = map[string]int{
	EncodingTypeNone: 9,
	EncodingTypeZstd: 1,
}

func GetPreferredEncodingType(acceptEncodingHeader []string) string {
	encodingType := EncodingTypeNone

	for _, v := range acceptEncodingHeader {
		for _, vv := range strings.Split(v, ",") {
			vv = strings.TrimSpace(vv)
			priority, ok := EncodingPriorMap[vv]
			if ok && priority < EncodingPriorMap[encodingType] {
				encodingType = vv
			}
		}
	}

	return encodingType
}

func IsAvailableEncoding(encodingType *string) bool {
	if encodingType == nil {
		return false
	}

	if *encodingType == "" {
		*encodingType = EncodingTypeNone
		return true
	}

	_, ok := EncodingPriorMap[*encodingType]
	return ok
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

func NopWriteCloser(writer io.Writer) io.WriteCloser {
	return nopWriteCloser{writer}
}

type wrapCloser struct {
	WrapClose func() error
}

func (wrapCloser wrapCloser) Close() error {
	return wrapCloser.WrapClose()
}

func WrapCloser(close func() error) io.Closer {
	return wrapCloser{close}
}

type EncodingReadCloser struct {
	io.Reader
	io.Closer
}

func NewEncodingReadCloser(reader io.ReadCloser, encodingType string) io.ReadCloser {
	switch encodingType {
	case EncodingTypeZstd:
		zstdReadCloser := gozstd.NewReader(reader)
		return &EncodingReadCloser{
			Reader: zstdReadCloser,
			Closer: WrapCloser(func() error {
				zstdReadCloser.Release()
				return reader.Close()
			}),
		}
	default:
		return reader
	}
}

func NewBufferEncodingReader(data []byte, encodingType string) (io.Reader, int64, error) {
	switch encodingType {
	case EncodingTypeZstd:
		compressedData, err := func() (dst []byte, err error) {
			// Recover from panic caused by gozstd.CompressLevel
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("%v", r)
				}
			}()

			dst = gozstd.CompressLevel([]byte{}, data, 1)
			return
		}()
		return bytes.NewBuffer(compressedData), int64(len(compressedData)), err
	}
	return bytes.NewBuffer(data), int64(len(data)), nil
}

func NewEncodingWriteCloser(writer io.Writer, encodingType string) io.WriteCloser {
	switch encodingType {
	case EncodingTypeZstd:
		return gozstd.NewWriterLevel(writer, 1)
	}

	return NopWriteCloser(writer)
}
