// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func Test_completed_ServeHTTP(t *testing.T) {
	tests := []struct {
		name         string
		isCompleted  bool
		statusCode   int
		expectedBody string
	}{
		{
			name:         "test OK",
			isCompleted:  true,
			statusCode:   http.StatusOK,
			expectedBody: "OK",
		},
		{
			name:         "test Downloading",
			isCompleted:  false,
			statusCode:   http.StatusNotFound,
			expectedBody: "Downloading",
		},
	}

	server := CreateHttpServer(&mainContext)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/completed", nil)
			responseRecorder := httptest.NewRecorder()

			mainContext.Completed.Store(tt.isCompleted)

			server.Handler.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != tt.statusCode {
				t.Errorf("Want status '%d', got '%d'", tt.statusCode, responseRecorder.Code)
			}

			if responseRecorder.Body.String() != tt.expectedBody {
				t.Errorf("Want Body '%s', got '%s'", tt.expectedBody, responseRecorder.Body.String())
			}
		})
	}
}

func Test_uuid_ServeHTTP(t *testing.T) {
	tests := []struct {
		name       string
		uuid       string
		statusCode int
	}{
		{
			name:       "test uuid 1",
			uuid:       uuid.New().String(),
			statusCode: http.StatusOK,
		},
		{
			name:       "test uuid 2",
			uuid:       uuid.New().String(),
			statusCode: http.StatusOK,
		},
		{
			name:       "test uuid 3",
			uuid:       uuid.New().String(),
			statusCode: http.StatusOK,
		},
	}

	server := CreateHttpServer(&mainContext)
	base_uuid := uuid.New().String()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/uuid", nil)
			responseRecorder := httptest.NewRecorder()

			mainContext.UUID = tt.uuid

			server.Handler.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != tt.statusCode {
				t.Errorf("Want status '%d', got '%d'", tt.statusCode, responseRecorder.Code)
			}

			if responseRecorder.Body.String() != tt.uuid {
				t.Errorf("Want uuid '%s', got '%s'", tt.uuid, responseRecorder.Body.String())
			}

			if base_uuid == tt.uuid {
				t.Errorf("uuid not changed '%s'", tt.uuid)
			}
		})
	}
}

func Test_version_ServeHTTP(t *testing.T) {
	tests := []struct {
		name         string
		BuildVersion string
		statusCode   int
	}{
		{
			name:         "test version 1",
			BuildVersion: "123",
			statusCode:   http.StatusOK,
		},
		{
			name:         "test version 2",
			BuildVersion: "456",
			statusCode:   http.StatusOK,
		},
	}

	server := CreateHttpServer(&mainContext)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/version", nil)
			responseRecorder := httptest.NewRecorder()

			BuildVersion = tt.BuildVersion

			server.Handler.ServeHTTP(responseRecorder, request)

			if responseRecorder.Code != tt.statusCode {
				t.Errorf("Want status '%d', got '%d'", tt.statusCode, responseRecorder.Code)
			}

			version := getVersion()
			if responseRecorder.Body.String() != version {
				t.Errorf("Want version '%s', got '%s'", version, responseRecorder.Body.String())
			}
		})
	}
}
