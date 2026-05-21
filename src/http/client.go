// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/naver/p2pcp/constants"
)

type HttpClient struct {
	client http.Client
}

type ContextReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *ContextReadCloser) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

func NewContextReadCloser(readCloser io.ReadCloser, cancel context.CancelFunc) *ContextReadCloser {
	return &ContextReadCloser{
		ReadCloser: readCloser,
		cancel:     cancel,
	}
}

func NewHttpClient(maxConnsPerHost int) *HttpClient {
	httpClient := &HttpClient{
		client: http.Client{
			Transport: &http.Transport{
				MaxConnsPerHost: maxConnsPerHost,
				IdleConnTimeout: constants.HttpIdleConnTimeout,
				Dial: (&net.Dialer{
					Timeout: constants.HttpConnectTimeout,
				}).Dial,
			},
		},
	}

	return httpClient
}

func checkContentType(resp *http.Response, expected string) error {
	if expected == "" {
		return nil
	}

	contentTypeHeaders, ok := resp.Header["Content-Type"]
	if !ok || len(contentTypeHeaders) == 0 {
		return errors.New("Content-Type header not found")
	}

	for _, contentTypeHeader := range contentTypeHeaders {
		tokens := strings.Split(contentTypeHeader, ";")
		for _, token := range tokens {
			if strings.TrimSpace(token) == expected {
				return nil
			}
		}
	}

	return fmt.Errorf("no Content-Type matched (expected:%s, actual:%v)", expected, contentTypeHeaders)
}

func (httpClient *HttpClient) request(ctx context.Context, url string, options ...Option) (http.Header, io.ReadCloser, error) {
	cfg := config{
		method:             "GET",
		body:               nil,
		acceptEncodingType: EncodingTypeNone,
	}

	for _, option := range options {
		option(&cfg)
	}

	request, err := http.NewRequestWithContext(ctx, cfg.method, url, cfg.body)
	if err != nil {
		return nil, nil, err
	}

	if cfg.wantDigest != "" {
		request.Header.Set("Want-Digest", cfg.wantDigest)
	}

	if cfg.contentType != "" {
		request.Header.Set("Content-Type", cfg.contentType)
	}

	switch cfg.acceptEncodingType {
	case EncodingTypeZstd:
		request.Header.Set("Accept-Encoding", EncodingTypeZstd)
	}

	response, err := httpClient.client.Do(request)
	if err != nil {
		return nil, nil, err
	}

	// Closing of response.Body should be done at the calling function.
	// It is only closed within this function in case of an error.

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		response.Body.Close()
		return nil, nil, fmt.Errorf("HTTP response status is not success (%s)", response.Status)
	}

	err = checkContentType(response, cfg.expectedContentType)
	if err != nil {
		response.Body.Close()
		return nil, nil, err
	}

	// Use only the first value.
	encodingType := strings.TrimSpace(response.Header.Get("Content-Encoding"))

	return response.Header, NewEncodingReadCloser(response.Body, encodingType), nil
}

func (httpClient *HttpClient) RequestWithContext(ctx context.Context, url string, options ...Option) (http.Header, io.ReadCloser, error) {
	return httpClient.request(ctx, url, options...)
}

func (httpClient *HttpClient) RequestBodyWithContext(ctx context.Context, url string, options ...Option) ([]byte, error) {
	_, reader, err := httpClient.request(ctx, url, options...)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

func (httpClient *HttpClient) Request(url string, options ...Option) (http.Header, io.ReadCloser, error) {
	ctx, cancel := context.WithTimeout(context.Background(), constants.PeerTransferTimeout)

	header, readCloser, err := httpClient.request(ctx, url, options...)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	return header, NewContextReadCloser(readCloser, cancel), nil
}

func (httpClient *HttpClient) RequestBody(url string, options ...Option) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), constants.PeerTransferTimeout)
	defer cancel()
	return httpClient.RequestBodyWithContext(ctx, url, options...)
}

var httpClientInstance *HttpClient
var httpClientOnce sync.Once

func GetDefaultHttpClient() *HttpClient {
	httpClientOnce.Do(func() {
		httpClientInstance = NewHttpClient(constants.DefaultMaxConnsPerHost)
	})
	return httpClientInstance
}

func RequestWithContext(ctx context.Context, url string, options ...Option) ([]byte, error) {
	return GetDefaultHttpClient().RequestBodyWithContext(ctx, url, options...)
}

func Request(url string, options ...Option) ([]byte, error) {
	return GetDefaultHttpClient().RequestBody(url, options...)
}
