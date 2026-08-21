#!/usr/bin/env sh
# NOTICE generator. Unions dependencies across every target architecture we ship
# so no attribution is missed. go-licenses analyzes a single GOARCH per run, so we
# run it per arch and merge the results by module.
set -eu

# Architectures the image is published for. Keep in sync with the release pipeline platforms.
ARCHES="${ARCHES:-amd64 arm64}"
GO_LICENSES_VERSION="${GO_LICENSES_VERSION:-v1.6.0}"
GO="${GO:-go}"

cd "$(dirname "$0")/../src"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# Build go-licenses for the host architecture.
GOBIN="$TMP" "$GO" install "github.com/google/go-licenses@${GO_LICENSES_VERSION}"

# Collect dependencies per arch as CSV (Name,URL,License).
: > "$TMP/rows"
for arch in $ARCHES; do
	GOOS=linux GOARCH="$arch" CGO_ENABLED=1 "$TMP/go-licenses" csv ./... \
		--ignore github.com/naver/p2pcp >> "$TMP/rows" 2>/dev/null
done
sort -u "$TMP/rows" > "$TMP/union"

cat <<'EOF'
p2pcp
Copyright 2019-2026 NAVER Corp.

This product includes software developed at
NAVER Corp. (https://www.navercorp.com/).


-------------------------------------------------------------------------------
The following components are included in the p2pcp binary:
-------------------------------------------------------------------------------
EOF

awk -F',' '{ printf "\n%s\nLicensed under the %s License\n%s\n", $1, $3, $2 }' "$TMP/union"

cat <<'EOF'

facebook/zstd (statically linked via gozstd v1.23.2)
Dual-licensed under BSD-3-Clause OR GPLv2 (BSD-3-Clause selected)
https://github.com/facebook/zstd


-------------------------------------------------------------------------------
The following components are included in the container image
(ghcr.io/naver/p2pcp) in addition to the components listed above.
They are not part of the binary distribution.
-------------------------------------------------------------------------------

EOF

# Base image attribution, supplied by the open source review team. Replace this
# block verbatim when the base image changes; do not hand-edit individual entries.
cat <<'EOF'
Base Image

Image: alpine:3.21
Digest: sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d
Download source: docker.io/library/alpine

The following components are included in this base image:

alpine-baselayout (3.6.8-r1)
Licensed under the GPL-2.0-only License
https://gitlab.alpinelinux.org/alpine/aports/-/tree/5f0cd7890349e7fe11128478ac506c709805224d/main/alpine-baselayout

alpine-baselayout-data (3.6.8-r1)
Licensed under the GPL-2.0-only License
https://gitlab.alpinelinux.org/alpine/aports/-/tree/5f0cd7890349e7fe11128478ac506c709805224d/main/alpine-baselayout

alpine-keys (2.5-r0)
Licensed under the MIT License
https://gitlab.alpinelinux.org/alpine/aports/-/tree/3.21-stable/main/alpine-keys

alpine-release (3.21.7-r0)
Licensed under the MIT License
https://gitlab.alpinelinux.org/alpine/aports/-/tree/d0fb4afb42fe848ef523e006f2672ef2caf75ee1/main/alpine-base

apk-tools (2.14.6-r3)
Licensed under the GPL-2.0-only License
https://gitlab.alpinelinux.org/alpine/apk-tools/-/tags/v2.14.6

busybox (1.37.0-r14)
Licensed under the GPL-2.0-only License
https://gitlab.alpinelinux.org/alpine/aports/-/tree/284d827a2793963e6dcf88324bd1eb81adff579c/main/busybox

busybox-binsh (1.37.0-r14)
Licensed under the GPL-2.0-only License
https://gitlab.alpinelinux.org/alpine/aports/-/tree/284d827a2793963e6dcf88324bd1eb81adff579c/main/busybox

ssl_client (1.37.0-r14)
Licensed under the GPL-2.0-only License
https://gitlab.alpinelinux.org/alpine/aports/-/tree/284d827a2793963e6dcf88324bd1eb81adff579c/main/busybox

ca-certificates (20260413-r0)
Licensed under MPL-2.0 AND MIT
https://gitlab.alpinelinux.org/alpine/aports/-/tree/f4e20cbe4e1a935579988dfd8cae241d8d050a63/main/ca-certificates

ca-certificates-bundle (20260413-r0)
Licensed under MPL-2.0 AND MIT
https://gitlab.alpinelinux.org/alpine/aports/-/tree/f4e20cbe4e1a935579988dfd8cae241d8d050a63/main/ca-certificates

libcrypto3 (3.3.7-r0)
Licensed under the Apache-2.0 License
https://gitlab.alpinelinux.org/alpine/aports/-/tree/8ec72d9972fac0f527a00fd5ee927f88f0895cac/main/openssl

libssl3 (3.3.7-r0)
Licensed under the Apache-2.0 License
https://gitlab.alpinelinux.org/alpine/aports/-/tree/8ec72d9972fac0f527a00fd5ee927f88f0895cac/main/openssl

musl (1.2.5-r11)
Licensed under the MIT License
https://gitlab.alpinelinux.org/alpine/aports/-/tree/21cafa1183cf7501d3f1744994d221c038436e45/main/musl

musl-utils (1.2.5-r11)
Licensed under MIT AND BSD-2-Clause AND GPL-2.0-or-later
https://gitlab.alpinelinux.org/alpine/aports/-/tree/21cafa1183cf7501d3f1744994d221c038436e45/main/musl

scanelf (1.3.8-r1)
Licensed under the GPL-2.0-only License
https://gitlab.alpinelinux.org/alpine/aports/-/tree/398a5aee3025ec8a4d0d761e448dc86ac777fa09/main/pax-utils

zlib (1.3.2-r0)
Licensed under the Zlib License
https://gitlab.alpinelinux.org/alpine/aports/-/tree/2b38f55109add14f4f99a974c2fdf421b6b9e9e9/main/zlib
EOF
