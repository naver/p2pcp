#!/bin/bash
#
# Test P2P transfer with multiple peers and seeder configurations
#
# Usage: peer_test.sh <src> <dst>
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

PEER_COUNT=4

# Get ports for seeder + peers
PORTS=$(get_free_ports $((PEER_COUNT + 1))) || die "Failed to get free ports"
read -ra PORT_ARRAY <<< "$PORTS"
SEEDER_PORT="${PORT_ARRAY[0]}"

# Build peer list
PEER_LIST="127.0.0.1:${SEEDER_PORT}"
for ((i = 0; i < PEER_COUNT; i++)); do
    PEER_LIST+=",127.0.0.1:${PORT_ARRAY[$((i + 1))]}"
done

run_and_verify() {
    rm -rf "${DST}"
    mkdir -p "${DST}" || die "Failed to create destination directory"

    PEER_PIDS=()
    for ((i = 0; i < PEER_COUNT; i++)); do
        SUB_DST="${DST}/data_${i}"
        "${BASE_DIR}/p2pcp" -dst "${SUB_DST}" -listen-addr "127.0.0.1:${PORT_ARRAY[$((i + 1))]}" -peer-list "${PEER_LIST}" -exit-complete "$@" &
        PEER_PIDS+=($!)
    done

    local failed=0
    for pid in "${PEER_PIDS[@]}"; do
        if ! wait "${pid}"; then
            warn "Peer process ${pid} failed"
            failed=1
        fi
    done

    if [[ "$failed" -eq 1 ]]; then
        return 1
    fi

    for ((i = 0; i < PEER_COUNT; i++)); do
        SUB_DST="${DST}/data_${i}"
        SUB_DST_CHECKSUM=$("${BASE_DIR}/test/create_checksum.sh" "${SUB_DST}") || { warn "Failed to create checksum for ${SUB_DST}"; return 1; }
        verify_checksum "${SRC_CHECKSUM}" "${SUB_DST_CHECKSUM}" "${SUB_DST}" || return 1
    done

    return 0
}

info "Testing peer transfer with embedded source..."
run_and_verify -src "${SRC}" -compress-type zstd || exit 1

info "Starting seeder with custom chunk size on port ${SEEDER_PORT}..."
"${BASE_DIR}/p2pcp" -src "${SRC}" -listen-addr "127.0.0.1:${SEEDER_PORT}" -chunk-size 8192 &
SEEDER_PID=$!
trap 'kill_and_wait "${SEEDER_PID}"' EXIT

wait_for_http "http://127.0.0.1:${SEEDER_PORT}/version" 10 || {
    if ! is_running "${SEEDER_PID}"; then
        die "Seeder process died"
    fi
    die "Seeder failed to start (HTTP not ready after 10s)"
}

info "Testing peer transfer with external seeder..."
run_and_verify -compress-type zstd -chunk-size 8192 || exit 1

kill_and_wait "${SEEDER_PID}"
trap - EXIT

info "Starting seeder with default settings on port ${SEEDER_PORT}..."
"${BASE_DIR}/p2pcp" -src "${SRC}" -listen-addr "127.0.0.1:${SEEDER_PORT}" &
SEEDER_PID=$!
trap 'kill_and_wait "${SEEDER_PID}"' EXIT

wait_for_http "http://127.0.0.1:${SEEDER_PORT}/version" 10 || {
    if ! is_running "${SEEDER_PID}"; then
        die "Seeder process died"
    fi
    die "Seeder failed to start (HTTP not ready after 10s)"
}

info "Testing basic peer transfer..."
run_and_verify || exit 1

info "Testing with high concurrency..."
run_and_verify -peer-num-concurrent 8 -verify-on-complete -verbose -statistics || exit 1

info "Testing with peer and local concurrency..."
run_and_verify -src "${SRC}" -peer-num-concurrent 8 -local-num-concurrent 8 || exit 1

info "Testing with zstd compression..."
run_and_verify -src "${SRC}" -compress-type zstd || exit 1

info "Success"
