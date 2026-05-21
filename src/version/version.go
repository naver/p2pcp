// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package version

import "fmt"

// Variables are set during build. See Makefile for details.
var MajorVersion string
var MinorVersion string
var PatchVersion string
var BuildVersion string

func GetVersion() string {
	return fmt.Sprintf("p2pcp-%s.%s.%s+%s", MajorVersion, MinorVersion, PatchVersion, BuildVersion)
}
