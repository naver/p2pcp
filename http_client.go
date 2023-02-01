// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type HttpRequestOptions struct {
	ExpectedContentType string
	EncodingType        string
}

const DefaultMaxConnsPerHost = 2

func NewHttpRequestOptions() HttpRequestOptions {
	return HttpRequestOptions{
		EncodingType: EncodingTypeNone,
	}
}

type HttpClient struct {
	client           http.Client
	maxContentLength int64
	retries          int
}

func NewHttpClient(timeout time.Duration, maxConnsPerHost int) *HttpClient {
	h := &HttpClient{
		client: http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxConnsPerHost: maxConnsPerHost,
				IdleConnTimeout: 5 * time.Second,
				Dial: (&net.Dialer{
					KeepAlive: 5 * time.Second,
					Timeout:   HttpConnectTimeout,
				}).Dial,
			},
		},
		maxContentLength: MaxChunkSize,
		retries:          1,
	}

	return h
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

	return fmt.Errorf("no content type matched. expected:%v, actual:%v", expected, contentTypeHeaders)
}

func (h *HttpClient) requestHttpAsync(url string, options HttpRequestOptions, retries int) (io.ReadCloser, error) {
	var err error

	for i := 0; i < retries; i++ {
		var request *http.Request
		request, err = http.NewRequest("GET", url, nil)
		if err != nil {
			continue
		}

		switch options.EncodingType {
		case EncodingTypeZstd:
			request.Header.Set("Accept-Encoding", EncodingTypeZstd)
		}

		var response *http.Response
		response, err = h.client.Do(request)
		if err != nil {
			continue
		}
		// requestHttpAsync 함수는 response.Body 의 read 와 close 를 return 받은 함수에서 해야함
		// 따라서 에러가 발생한 경우에만 여기서 close 함

		if response.StatusCode < 200 || response.StatusCode >= 300 {
			err = fmt.Errorf("HTTP response status is not success.:%s", response.Status)
			response.Body.Close()
			continue
		}

		err = checkContentType(response, options.ExpectedContentType)
		if err != nil {
			response.Body.Close()
			continue
		}

		/* 첫 번째 값만 허용 */
		encodingType := strings.TrimSpace(response.Header.Get("Content-Encoding"))

		if response.ContentLength > h.maxContentLength {
			err = fmt.Errorf("content Length exceeded. max:%v, response:%v", h.maxContentLength, response.ContentLength)
			response.Body.Close()
			continue
		}

		return NewEncodingReadCloser(response.Body, encodingType), nil
	}

	return nil, err
}

func (h *HttpClient) RequestHttpAsync(url string, options HttpRequestOptions) (io.ReadCloser, error) {
	return h.requestHttpAsync(url, options, h.retries)
}

func (h *HttpClient) RequestHttp(url string, options HttpRequestOptions) ([]byte, error) {
	reader, err := h.RequestHttpAsync(url, options)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

func RequestHttp(url string, options HttpRequestOptions) ([]byte, error) {
	return GetDefaultHttpClient().RequestHttp(url, options)
}

var httpClientInstance *HttpClient
var httpClientOnce sync.Once

func GetDefaultHttpClient() *HttpClient {
	httpClientOnce.Do(func() {
		httpClientInstance = NewHttpClient(PeerTransferTimeout, DefaultMaxConnsPerHost)
	})
	return httpClientInstance
}
