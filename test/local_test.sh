#!/bin/bash
#
# Test local file transfer with various compression and concurrency options
#
# Usage: local_test.sh <src> <dst>
#

CURR_DIR=$(pwd)
source "$(dirname "$0")/common.sh"

if [[ $# -lt 2 ]]; then
    echo "Usage: $0 <src> <dst>"
    echo "  src: source path"
    echo "  dst: destination path"
    exit 1
fi

SRC="${CURR_DIR}/$1"
DST="${CURR_DIR}/$2"

if [[ ! -d "$SRC" ]]; then
    die "Source directory does not exist: $SRC"
fi

SRC_CHECKSUM=$("${BASE_DIR}/test/create_checksum.sh" "${SRC}") || die "Failed to create source checksum"

LISTEN_PORT=$(get_free_port) || die "Failed to get free port"

run_and_verify() {
    rm -rf "${DST}"

    "${BASE_DIR}/p2pcp" -listen-addr "127.0.0.1:${LISTEN_PORT}" -exit-complete "$@" || { warn "p2pcp execution failed"; return 1; }

    local dst_checksum
    dst_checksum=$("${BASE_DIR}/test/create_checksum.sh" "${DST}") || { warn "Failed to create destination checksum"; return 1; }

    verify_checksum "${SRC_CHECKSUM}" "${dst_checksum}" || return 1

    return 0
}

info "Testing basic transfer..."
run_and_verify -src "${SRC}" -dst "${DST}" || exit 1

info "Testing with local: prefix..."
run_and_verify -src "local:${SRC}" -dst "local:${DST}" || exit 1

info "Testing with zstd compression..."
run_and_verify -src "${SRC}" -dst "${DST}" -compress-type zstd || exit 1

info "Testing with custom chunk size and concurrency..."
run_and_verify -src "${SRC}" -dst "${DST}" -compress-type none -chunk-size 8192 -local-num-concurrent 4 -verify-on-complete -verbose || exit 1

info "Testing with zstd, custom chunk size and high concurrency..."
run_and_verify -src "${SRC}" -dst "${DST}" -compress-type zstd -chunk-size 8192 -local-num-concurrent 8 -verify-on-complete -verbose -statistics || exit 1

info "Success"
