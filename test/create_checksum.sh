#!/bin/bash
#
# Create a checksum for a directory (including file permissions and contents)
#
# Usage: create_checksum.sh <target_dir>
# Output: SHA1 checksum string to stdout
#

set -euo pipefail

if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <target_dir>" >&2
    exit 1
fi

TARGET_DIR="$1"

if [[ ! -d "$TARGET_DIR" ]]; then
    echo "Error: Directory does not exist: $TARGET_DIR" >&2
    exit 1
fi

tmp=$(mktemp /tmp/p2pcp-checksum.XXXXXX) || { echo "Error: Failed to create temp file" >&2; exit 1; }
trap 'rm -f "$tmp"' EXIT

cd "$TARGET_DIR" || { echo "Error: Failed to cd to $TARGET_DIR" >&2; exit 1; }

# Include file metadata (permissions, names)
find . -ls | awk '{printf("%s ",$3); for(i=11; i<=NF; i++){printf("%s ",$i)}; printf("\n");}' | tail -n +2 | sort > "$tmp"
# Include file contents
find . -type f -exec sha1sum {} \; | sort >> "$tmp"

sha1sum "$tmp" | awk '{print $1}'
