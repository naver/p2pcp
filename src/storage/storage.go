// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package storage

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/naver/p2pcp/types"
)

type Storage interface {
	GetBaseDirectory() string

	Walk(path string, fn func(types.Entry) error) error
	Prepare() error
	PrepareEntry(entry types.Entry) error
	CommitEntry(entry types.Entry) error

	Write(name string, offset int64, size int64, reader io.Reader) (int64, error)
	GetReader(name string, offset int64, size int64) (io.ReadCloser, error)
}

type Factory struct {
	storageType StorageType
	storagePath string
	rawURL      string
}

func NewFactory(rawURL string) (*Factory, error) {
	if rawURL == "" {
		return &Factory{storageType: StorageTypeNone}, nil
	}

	storageType := StorageTypeLocal
	path := rawURL
	parts := strings.SplitN(rawURL, ":", 2)
	if len(parts) == 2 {
		var err error
		storageType, err = StorageTypeFrom(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid storage type (%s)", rawURL)
		}
		path = parts[1]
	}

	if path != "" {
		normalized, err := filepath.Abs(path)
		if err != nil {
			path = strings.TrimSuffix(strings.TrimSpace(path), "/")
		} else {
			path = normalized
		}
	}

	return &Factory{
		storageType: storageType,
		storagePath: path,
		rawURL:      rawURL,
	}, nil
}

func (f *Factory) Create(options ...Option) (Storage, error) {
	config := Config{
		path: f.storagePath,
	}
	for _, option := range options {
		option(&config)
	}

	switch f.storageType {
	case StorageTypeNone:
		// seed-only mode: no destination storage needed
		return nil, nil
	case StorageTypeLocal:
		return newLocalStorage(config.path)
	case StorageTypeAppdepot:
		return nil, fmt.Errorf("appdepot storage is not supported")
	case StorageTypeS3:
		return nil, fmt.Errorf("s3 storage is not supported")
	default:
		return nil, fmt.Errorf("invalid storage type: %s", f.storageType)
	}
}
