// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package version

import (
	"fmt"
	"testing"
)

func TestGetVersion(t *testing.T) {
	originalMajor := MajorVersion
	originalMinor := MinorVersion
	originalPatch := PatchVersion
	originalBuild := BuildVersion

	defer func() {
		MajorVersion = originalMajor
		MinorVersion = originalMinor
		PatchVersion = originalPatch
		BuildVersion = originalBuild
	}()

	tests := []struct {
		name  string
		major string
		minor string
		patch string
		build string
	}{
		{"normal", "1", "2", "3", "abc123"},
		{"empty", "", "", "", ""},
		{"zero_version", "0", "0", "0", "dev"},
		{"large_version", "10", "20", "30", "build9999"},
		{"partial_empty", "1", "", "3", ""},
		{"long_build", "1", "0", "0", "abcdef1234567890"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			MajorVersion = tt.major
			MinorVersion = tt.minor
			PatchVersion = tt.patch
			BuildVersion = tt.build

			version := GetVersion()
			// Expected format: "p2pcp-{major}.{minor}.{patch}+{build}"
			expected := fmt.Sprintf("p2pcp-%s.%s.%s+%s",
				tt.major, tt.minor, tt.patch, tt.build)

			if version != expected {
				t.Errorf("Expected '%s', got '%s'", expected, version)
			}
		})
	}
}
