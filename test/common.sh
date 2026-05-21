#!/bin/bash
#
# Common utilities for p2pcp test scripts
#
# Usage:
#   source "$(dirname "$0")/common.sh"
#

set -euo pipefail

# Resolve BASE_DIR (project root) from common.sh location
BASE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export BASE_DIR

# Colors for output (disabled if not a terminal)
if [[ -t 1 ]]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    NC='\033[0m' # No Color
else
    RED=''
    GREEN=''
    YELLOW=''
    NC=''
fi

#######################################
# Print error message and exit
#######################################
die() {
    echo -e "${RED}Error: $1${NC}" >&2
    exit 1
}

#######################################
# Print info message
#######################################
info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

#######################################
# Print warning message
#######################################
warn() {
    echo -e "${YELLOW}[WARN]${NC} $1" >&2
}

#######################################
# Get a free port that is not in use
#######################################
get_free_port() {
    local port
    for _ in {1..100}; do
        port=$((RANDOM % 10000 + 20000))
        if ! nc -z 127.0.0.1 "$port" 2>/dev/null; then
            echo "$port"
            return 0
        fi
    done
    return 1
}

#######################################
# Get multiple free ports
# Arguments:
#   $1 - Number of ports needed
# Outputs:
#   Space-separated list of free ports
#######################################
get_free_ports() {
    local count=${1:-1}
    local ports=()
    local port

    for _ in $(seq 1 "$count"); do
        for _ in {1..100}; do
            port=$((RANDOM % 10000 + 20000))
            if ! nc -z 127.0.0.1 "$port" 2>/dev/null; then
                # Check if port is already in our list
                local duplicate=false
                for p in "${ports[@]:-}"; do
                    if [[ "$p" == "$port" ]]; then
                        duplicate=true
                        break
                    fi
                done
                if [[ "$duplicate" == "false" ]]; then
                    ports+=("$port")
                    break
                fi
            fi
        done
    done

    if [[ ${#ports[@]} -ne $count ]]; then
        return 1
    fi

    echo "${ports[*]}"
    return 0
}

#######################################
# Wait for HTTP endpoint to return 200
# Arguments:
#   $1 - URL to check
#   $2 - Timeout in seconds (default: 30)
#######################################
wait_for_http() {
    local url=$1
    local timeout=${2:-30}

    for ((i = 0; i < timeout; i++)); do
        local code
        code=$(curl -o /dev/null -s -w "%{http_code}" "$url") || true
        if [[ "$code" == "200" ]]; then
            return 0
        fi
        sleep 1
    done
    return 1
}

#######################################
# Verify checksums match
# Arguments:
#   $1 - Expected checksum
#   $2 - Actual checksum
#   $3 - Description (optional)
#######################################
verify_checksum() {
    local expected=$1
    local actual=$2
    local desc=${3:-""}

    if [[ "$expected" != "$actual" ]]; then
        if [[ -n "$desc" ]]; then
            echo "Error: Checksum mismatch for $desc" >&2
        else
            echo "Error: Checksum mismatch" >&2
        fi
        echo "  Expected: $expected" >&2
        echo "  Actual:   $actual" >&2
        return 1
    fi
    return 0
}

#######################################
# Kill process and wait for termination
# Arguments:
#   $1 - PID to kill
#   $2 - Timeout in seconds (default: 5)
#######################################
kill_and_wait() {
    local pid=$1
    local timeout=${2:-5}

    if ! kill -0 "$pid" 2>/dev/null; then
        return 0
    fi

    kill -TERM "$pid" 2>/dev/null || true

    for ((i = 0; i < timeout * 10; i++)); do
        if ! kill -0 "$pid" 2>/dev/null; then
            return 0
        fi
        sleep 0.1
    done

    # Force kill if still running
    kill -KILL "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
    return 0
}

#######################################
# Check if a process is still running
#######################################
is_running() {
    local pid=$1
    kill -0 "$pid" 2>/dev/null
}
