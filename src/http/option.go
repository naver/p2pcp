// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package http

import "io"

type Option func(*config)

type config struct {
	method              string
	body                io.Reader
	contentType         string
	expectedContentType string
	acceptEncodingType  string
	wantDigest          string
}

func WithMethod(method string) Option {
	return func(c *config) {
		c.method = method
	}
}

func WithBody(body io.Reader) Option {
	return func(c *config) {
		c.body = body
	}
}

func WithContentType(contentType string) Option {
	return func(c *config) {
		c.contentType = contentType
	}
}

func WithExpectedContentType(expectedContentType string) Option {
	return func(c *config) {
		c.expectedContentType = expectedContentType
	}
}

func WithAcceptEncodingType(encodingType string) Option {
	return func(c *config) {
		c.acceptEncodingType = encodingType
	}
}

func WithWantDigest(wantDigest string) Option {
	return func(c *config) {
		c.wantDigest = wantDigest
	}
}
