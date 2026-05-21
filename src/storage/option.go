// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package storage

import (
	"fmt"
	"strings"
)

type StorageType string

const (
	StorageTypeNone     StorageType = "none"
	StorageTypeLocal    StorageType = "local"
	StorageTypeAppdepot StorageType = "appdepot"
	StorageTypeS3       StorageType = "s3"
)

func StorageTypeFrom(s string) (StorageType, error) {
	st := StorageType(strings.ToLower(s))
	switch st {
	case StorageTypeLocal, StorageTypeAppdepot, StorageTypeS3:
		return st, nil
	}
	return StorageTypeNone, fmt.Errorf("invalid storage type: %s", s)
}

type Option func(*Config)

type Config struct {
	path string

	appdepotURL     string
	appdepotAppName string
	appdepotEnvName string

	s3URL       string
	s3Region    string
	s3AccessKey string
	s3SecretKey string
}

func WithPath(path string) Option {
	return func(c *Config) {
		c.path = path
	}
}

func WithS3Config(url, region, accessKey, secretKey string) Option {
	return func(c *Config) {
		c.s3URL = url
		c.s3Region = region
		c.s3AccessKey = accessKey
		c.s3SecretKey = secretKey
	}
}

func WithAppdepotConfig(url, appName, envName string) Option {
	return func(c *Config) {
		c.appdepotURL = url
		c.appdepotAppName = appName
		c.appdepotEnvName = envName
	}
}
