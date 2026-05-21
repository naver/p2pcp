// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package http

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewContextReadCloser(t *testing.T) {
	cancelled := false
	cancel := func() {
		cancelled = true
	}

	reader := io.NopCloser(strings.NewReader("test"))
	contextReader := NewContextReadCloser(reader, cancel)

	if contextReader == nil {
		t.Fatal("Expected non-nil context reader")
	}

	// Read data
	data, err := io.ReadAll(contextReader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(data) != "test" {
		t.Errorf("Expected 'test', got '%s'", data)
	}

	// Close should call cancel
	err = contextReader.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
	if !cancelled {
		t.Error("Expected cancel to be called on Close")
	}
}

func TestNewHttpClient(t *testing.T) {
	client := NewHttpClient(10)
	if client == nil {
		t.Fatal("Expected non-nil client")
	}
}

func TestCheckContentType(t *testing.T) {
	tests := []struct {
		name     string
		headers  http.Header
		expected string
		wantErr  bool
	}{
		{
			name:     "no expected type",
			headers:  http.Header{},
			expected: "",
			wantErr:  false,
		},
		{
			name:     "matching content type",
			headers:  http.Header{"Content-Type": []string{"application/json"}},
			expected: "application/json",
			wantErr:  false,
		},
		{
			name:     "matching content type with charset",
			headers:  http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
			expected: "text/plain",
			wantErr:  false,
		},
		{
			name:     "missing content type header",
			headers:  http.Header{},
			expected: "application/json",
			wantErr:  true,
		},
		{
			name:     "non-matching content type",
			headers:  http.Header{"Content-Type": []string{"text/html"}},
			expected: "application/json",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{Header: tt.headers}
			err := checkContentType(resp, tt.expected)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkContentType() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHttpClient_Request(t *testing.T) {
	t.Run("successful GET request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("response body"))
		}))
		defer server.Close()

		client := NewHttpClient(10)
		header, reader, err := client.Request(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer reader.Close()

		if header == nil {
			t.Error("Expected non-nil header")
		}

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}
		if string(data) != "response body" {
			t.Errorf("Expected 'response body', got '%s'", data)
		}
	})

	t.Run("POST request with body", func(t *testing.T) {
		receivedBody := ""
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("Expected POST, got %s", r.Method)
			}
			body, _ := io.ReadAll(r.Body)
			receivedBody = string(body)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHttpClient(10)
		requestBody := bytes.NewReader([]byte("request data"))
		_, reader, err := client.Request(server.URL, WithMethod("POST"), WithBody(requestBody))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer reader.Close()

		if receivedBody != "request data" {
			t.Errorf("Expected 'request data', got '%s'", receivedBody)
		}
	})

	t.Run("request with content type", func(t *testing.T) {
		receivedContentType := ""
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedContentType = r.Header.Get("Content-Type")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHttpClient(10)
		_, reader, err := client.Request(server.URL, WithContentType("application/json"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer reader.Close()

		if receivedContentType != "application/json" {
			t.Errorf("Expected 'application/json', got '%s'", receivedContentType)
		}
	})

	t.Run("request with zstd encoding", func(t *testing.T) {
		receivedEncoding := ""
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedEncoding = r.Header.Get("Accept-Encoding")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHttpClient(10)
		_, reader, err := client.Request(server.URL, WithAcceptEncodingType(EncodingTypeZstd))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer reader.Close()

		if receivedEncoding != EncodingTypeZstd {
			t.Errorf("Expected '%s', got '%s'", EncodingTypeZstd, receivedEncoding)
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client := NewHttpClient(10)
		_, _, err := client.Request(server.URL)
		if err == nil {
			t.Error("Expected error for 500 status")
		}
	})

	t.Run("content type validation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHttpClient(10)
		_, _, err := client.Request(server.URL, WithExpectedContentType("application/json"))
		if err == nil {
			t.Error("Expected error for content type mismatch")
		}
	})
}

func TestHttpClient_RequestBody(t *testing.T) {
	t.Run("successful request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("test response"))
		}))
		defer server.Close()

		client := NewHttpClient(10)
		body, err := client.RequestBody(server.URL)
		if err != nil {
			t.Fatalf("RequestBody failed: %v", err)
		}

		if string(body) != "test response" {
			t.Errorf("Expected 'test response', got '%s'", body)
		}
	})
}

func TestHttpClient_RequestWithContext(t *testing.T) {
	t.Run("successful request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("context response"))
		}))
		defer server.Close()

		client := NewHttpClient(10)
		ctx := context.Background()
		_, reader, err := client.RequestWithContext(ctx, server.URL)
		if err != nil {
			t.Fatalf("RequestWithContext failed: %v", err)
		}
		defer reader.Close()

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}
		if string(data) != "context response" {
			t.Errorf("Expected 'context response', got '%s'", data)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHttpClient(10)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		_, _, err := client.RequestWithContext(ctx, server.URL)
		if err == nil {
			t.Error("Expected error due to context timeout")
		}
	})
}

func TestHttpClient_RequestBodyWithContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("body with context"))
	}))
	defer server.Close()

	client := NewHttpClient(10)
	ctx := context.Background()
	body, err := client.RequestBodyWithContext(ctx, server.URL)
	if err != nil {
		t.Fatalf("RequestBodyWithContext failed: %v", err)
	}

	if string(body) != "body with context" {
		t.Errorf("Expected 'body with context', got '%s'", body)
	}
}

func TestGetDefaultHttpClient(t *testing.T) {
	// GetDefaultHttpClient should return same instance (singleton)
	client1 := GetDefaultHttpClient()
	client2 := GetDefaultHttpClient()

	if client1 != client2 {
		t.Error("Expected GetDefaultHttpClient to return same instance")
	}

	if client1 == nil {
		t.Error("Expected non-nil client from GetDefaultHttpClient")
	}
}

func TestRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("global request"))
	}))
	defer server.Close()

	body, err := Request(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if string(body) != "global request" {
		t.Errorf("Expected 'global request', got '%s'", body)
	}
}

func TestRequestWithContext_Global(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("global context request"))
	}))
	defer server.Close()

	ctx := context.Background()
	body, err := RequestWithContext(ctx, server.URL)
	if err != nil {
		t.Fatalf("RequestWithContext failed: %v", err)
	}

	if string(body) != "global context request" {
		t.Errorf("Expected 'global context request', got '%s'", body)
	}
}

func TestHttpClient_WantDigest(t *testing.T) {
	receivedDigest := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedDigest = r.Header.Get("Want-Digest")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewHttpClient(10)
	_, reader, err := client.Request(server.URL, WithWantDigest("xxh3"))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer reader.Close()

	if receivedDigest != "xxh3" {
		t.Errorf("Expected 'xxh3', got '%s'", receivedDigest)
	}
}
