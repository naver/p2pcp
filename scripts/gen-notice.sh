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


The following components are included in this product:
EOF

awk -F',' '{ printf "\n%s\nLicensed under the %s License\n%s\n", $1, $3, $2 }' "$TMP/union"

cat <<'EOF'

facebook/zstd (statically linked via gozstd v1.23.2)
Dual-licensed under BSD-3-Clause OR GPLv2 (BSD-3-Clause selected)
https://github.com/facebook/zstd
EOF
