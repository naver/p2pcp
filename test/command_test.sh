#!/bin/bash
#
# Test execute-on-complete and execute-on-each-file-complete callbacks
#
# Usage: command_test.sh <src> <dst>
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

SEEDER_PORT=$(get_free_port) || die "Failed to get free port"
PEER_PORT=$(get_free_port) || die "Failed to get free port"

info "Starting seeder on port ${SEEDER_PORT}..."
"${BASE_DIR}/p2pcp" -listen-addr "127.0.0.1:${SEEDER_PORT}" -src "${SRC}" -chunk-size 8192 &
SEEDER_PID=$!
trap 'kill_and_wait "${SEEDER_PID}"' EXIT

wait_for_http "http://127.0.0.1:${SEEDER_PORT}/version" 10 || {
    if ! is_running "${SEEDER_PID}"; then
        die "Seeder process died"
    fi
    die "Seeder failed to start (HTTP not ready after 10s)"
}

rm -rf "${DST}"
mkdir -p "${DST}" || die "Failed to create destination directory"
SUB_DST="${DST}/data"

info "Testing execute-on-complete and execute-on-each-file-complete..."
"${BASE_DIR}/p2pcp" -dst "${SUB_DST}" -listen-addr "127.0.0.1:${PEER_PORT}" -peer-list "127.0.0.1:${SEEDER_PORT}" -chunk-size 8192 -exit-complete \
    -execute-on-complete "echo -e '%{elapsed_time}\n%{file_count}\n%{total_file_size}' > ${DST}/on-complete" \
    -execute-on-each-file-complete "sha256sum %{file_path} | awk '{print \"%{file_path} %{file_size} \" \$1}' >> ${DST}/on-each-complete" || die "p2pcp peer execution failed"

info "Verifying checksum..."
DST_CHECKSUM=$("${BASE_DIR}/test/create_checksum.sh" "${SUB_DST}") || die "Failed to create destination checksum"
verify_checksum "${SRC_CHECKSUM}" "${DST_CHECKSUM}" "transfer result" || exit 1

kill_and_wait "${SEEDER_PID}"
trap - EXIT

info "Verifying file count and total size..."
# expected: calculated from filesystem (ground truth)
# actual: reported by p2pcp (value under test)
expected_files_count=$(find "${SRC}" -type f -size +0c | wc -l)
expected_total_size=$(find "${SRC}" -type f -size +0c -print0 | xargs -0 stat -c "%s" 2>/dev/null | awk '{total += $1} END {print total}')
actual_files_count=$(sed -n '2p' "${DST}/on-complete")
actual_total_size=$(sed -n '3p' "${DST}/on-complete")

if [[ "$actual_files_count" != "$expected_files_count" ]]; then
    die "p2pcp reported file count ($actual_files_count) does not match expected ($expected_files_count)"
fi

if [[ "$actual_total_size" != "$expected_total_size" ]]; then
    die "p2pcp reported total size ($actual_total_size) does not match expected ($expected_total_size)"
fi

info "Verifying per-file checksums..."
# expected: calculated from transferred files
# actual: reported by p2pcp via execute-on-each-file-complete
expected_checksum=$(find "${SUB_DST}" -type f -size +0c -exec sh -c 'echo "$(realpath "$1") $(stat -c "%s" "$1") $(sha256sum "$1" | cut -d " " -f1)"' _ {} \; | sort | sha256sum)
actual_checksum=$(sort "${DST}/on-each-complete" | sha256sum)

verify_checksum "${expected_checksum}" "${actual_checksum}" "per-file checksums" || exit 1

info "Success"
