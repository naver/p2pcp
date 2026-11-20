#!/bin/bash
#
# Test Kubernetes peer discovery (SRV/A records) using kind cluster
#
# Usage: k8s_test.sh
# Requires: Docker
#

source "$(dirname "$0")/common.sh"

KIND_VERSION="v0.27.0"
KUBECTL_VERSION="v1.32.0"
KIND="${BASE_DIR}/test/kind"
KUBECTL="${BASE_DIR}/test/kubectl"
KUBECONFIG="${BASE_DIR}/test/kind.kubeconfig"
CLUSTER_NAME="p2pcp-test"

#######################################
# Pinned SHA-256 of the official binaries; update together with KIND_VERSION and KUBECTL_VERSION
#######################################
kind_sha256() {
    case "$1" in
        linux-amd64)  echo a6875aaea358acf0ac07786b1a6755d08fd640f4c79b7a2e46681cc13f49a04b ;;
        linux-arm64)  echo 5e4507a41c69679562610b1be82ba4f80693a7826f4e9c6e39236169a3e4f9d0 ;;
        darwin-amd64) echo 3435134325b6b9406ccfec417b13bb46a808fc74e9a2ebb0ca31b379c8293863 ;;
        darwin-arm64) echo 5240ca1acb587e1d0386532dd8c3373d81f5173b5af322919fc56f0cdd646596 ;;
    esac
}

kubectl_sha256() {
    case "$1" in
        linux-amd64)  echo 646d58f6d98ee670a71d9cdffbf6625aeea2849d567f214bc43a35f8ccb7bf70 ;;
        linux-arm64)  echo ba4004f98f3d3a7b7d2954ff0a424caa2c2b06b78c17b1dccf2acc76a311a896 ;;
        darwin-amd64) echo 516585916f499077fac8c2fdd2a382818683f831020277472e6bcf8d1a6f9be4 ;;
        darwin-arm64) echo 5bfd5de53a054b4ef614c60748e28bf47441c7ed4db47ec3c19a3e2fa0eb5555 ;;
    esac
}

#######################################
# Print the host platform as <os>-<arch>, e.g. linux-amd64
#######################################
platform() {
    local os arch
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    arch=$(uname -m)
    [[ "$arch" == "x86_64" ]] && arch="amd64"
    [[ "$arch" == "aarch64" ]] && arch="arm64"
    echo "${os}-${arch}"
}

#######################################
# Print the SHA-256 of a file (sha256sum on Linux, shasum on macOS)
#######################################
sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | cut -d' ' -f1
    else
        shasum -a 256 "$1" | cut -d' ' -f1
    fi
}

#######################################
# Download a tool unless a copy with the pinned checksum is present
# Arguments: name, destination, URL, expected SHA-256
#######################################
ensure_tool() {
    local name="$1" dest="$2" url="$3" want="$4"
    if [[ -z "$want" ]]; then
        warn "no pinned checksum for ${name} on $(platform)"
        return 1
    fi
    if [[ -x "$dest" && "$(sha256_of "$dest")" == "$want" ]]; then
        info "${name} already exists, skipping download"
        return 0
    fi

    info "Downloading ${name}..."
    curl -fsSL -o "${dest}.tmp" "$url" || { rm -f "${dest}.tmp"; return 1; }
    if [[ "$(sha256_of "${dest}.tmp")" != "$want" ]]; then
        warn "checksum mismatch for ${url}"
        rm -f "${dest}.tmp"
        return 1
    fi
    chmod +x "${dest}.tmp"
    mv "${dest}.tmp" "$dest"
}

ensure_kind() {
    local p
    p=$(platform)
    ensure_tool "kind ${KIND_VERSION}" "$KIND" \
        "https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-${p}" "$(kind_sha256 "$p")"
}

ensure_kubectl() {
    local p
    p=$(platform)
    ensure_tool "kubectl ${KUBECTL_VERSION}" "$KUBECTL" \
        "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/${p%-*}/${p#*-}/kubectl" "$(kubectl_sha256 "$p")"
}

#######################################
# Run p2pcp in a pod and verify exit code
#######################################
run_and_verify() {
    local pod_name="$1"
    shift

    if ! "$KUBECTL" --kubeconfig "$KUBECONFIG" run -i "$pod_name" \
        --image=p2pcp-image:k8s-test \
        --image-pull-policy=IfNotPresent \
        --restart=Never \
        --namespace=p2pcp \
        --command -- /usr/bin/p2pcp "$@"; then
        warn "kubectl run failed for ${pod_name}"
        "$KUBECTL" --kubeconfig "$KUBECONFIG" delete pod "$pod_name" -n p2pcp 2>/dev/null || true
        return 1
    fi

    local exit_code
    exit_code=$("$KUBECTL" --kubeconfig "$KUBECONFIG" get pod "$pod_name" -n p2pcp -o jsonpath='{.status.containerStatuses[0].state.terminated.exitCode}' 2>/dev/null)

    local failed=0
    if [[ -z "${exit_code}" ]]; then
        warn "${pod_name} exit code not found (pod may not have terminated properly)"
        failed=1
    elif [[ "${exit_code}" -ne 0 ]]; then
        warn "${pod_name} failed with exit code ${exit_code}"
        failed=1
    fi

    if [[ "$failed" -eq 1 ]]; then
        warn "=== ${pod_name} logs ==="
        "$KUBECTL" --kubeconfig "$KUBECONFIG" logs "$pod_name" -n p2pcp 2>/dev/null || true
    fi

    "$KUBECTL" --kubeconfig "$KUBECONFIG" delete pod "$pod_name" -n p2pcp || warn "Failed to delete pod ${pod_name}"

    return "$failed"
}

#######################################
# Cleanup function
#######################################
cleanup() {
    info "Cleaning up..."
    "$KIND" delete cluster --name "$CLUSTER_NAME" --kubeconfig "$KUBECONFIG" 2>/dev/null || true
    rm -f "$KUBECONFIG"
}

# Main
ensure_kind || die "Failed to download kind"
ensure_kubectl || die "Failed to download kubectl"

# Clean up any existing cluster for idempotency
if "$KIND" get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    warn "Cluster '${CLUSTER_NAME}' already exists, deleting..."
    "$KIND" delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
    rm -f "$KUBECONFIG"
fi

info "Creating kind cluster..."
"$KIND" create cluster --name "$CLUSTER_NAME" --kubeconfig "$KUBECONFIG" || die "Failed to create kind cluster"
trap cleanup EXIT

info "Building p2pcp Docker image..."
docker build -t p2pcp-image:k8s-test "$BASE_DIR" || die "Failed to build Docker image"

info "Loading image to kind cluster..."
"$KIND" load docker-image p2pcp-image:k8s-test --name "$CLUSTER_NAME" || die "Failed to load image to kind"

info "Applying k8s manifests..."
"$KUBECTL" --kubeconfig "$KUBECONFIG" apply -f "${BASE_DIR}/test/k8s/p2pcp-namespace.yaml" || die "Failed to apply namespace"

info "Waiting for default service account..."
for ((i = 0; i < 30; i++)); do
    if "$KUBECTL" --kubeconfig "$KUBECONFIG" get serviceaccount default -n p2pcp &>/dev/null; then
        info "Default service account is ready"
        break
    fi
    sleep 1
done

if ! "$KUBECTL" --kubeconfig "$KUBECONFIG" get serviceaccount default -n p2pcp &>/dev/null; then
    die "Timeout waiting for default service account"
fi

"$KUBECTL" --kubeconfig "$KUBECONFIG" apply -f "${BASE_DIR}/test/k8s/p2pcp-seeder.yaml" || die "Failed to apply seeder"
"$KUBECTL" --kubeconfig "$KUBECONFIG" apply -f "${BASE_DIR}/test/k8s/p2pcp-seeder-service.yaml" || die "Failed to apply seeder service"

info "Waiting for seeder pod to be ready..."
"$KUBECTL" --kubeconfig "$KUBECONFIG" wait --for=condition=ready pod/p2pcp-seeder -n p2pcp --timeout=600s || die "Seeder pod not ready"

"$KUBECTL" --kubeconfig "$KUBECONFIG" apply -f "${BASE_DIR}/test/k8s/p2pcp-peer.yaml" || die "Failed to apply peer"
"$KUBECTL" --kubeconfig "$KUBECONFIG" apply -f "${BASE_DIR}/test/k8s/p2pcp-peer-service.yaml" || die "Failed to apply peer service"

info "Waiting for peer pods to be ready..."
replicas=$("$KUBECTL" --kubeconfig "$KUBECONFIG" get replicaset p2pcp-peer -n p2pcp -o jsonpath='{.spec.replicas}') || die "Failed to read peer replicas"
# Wait on the ReplicaSet; a pod selector only sees the pods that exist when the wait starts.
"$KUBECTL" --kubeconfig "$KUBECONFIG" wait --for=jsonpath='{.status.readyReplicas}'="$replicas" replicaset/p2pcp-peer -n p2pcp --timeout=600s || die "Peer pods not ready"

info "Testing with peer-list-srv..."
run_and_verify p2pcp-peer-srv \
    -peer-list-srv p2pcp-peer.p2pcp.svc.cluster.local \
    -dst /mnt/dst -verify-on-complete -verbose -exit-complete || exit 1

info "Testing with peer-list and peer-list-srv..."
run_and_verify p2pcp-peer-mixed-srv \
    -peer-list p2pcp-seeder:10090 \
    -peer-list-srv p2pcp-peer.p2pcp.svc.cluster.local \
    -dst /mnt/dst -verify-on-complete -compress-type zstd -verbose -exit-complete || exit 1

info "Testing with peer-list-a..."
run_and_verify p2pcp-peer-a \
    -peer-list-a p2pcp-peer.p2pcp.svc.cluster.local \
    -dst /mnt/dst -verify-on-complete -compress-type zstd -verbose -exit-complete || exit 1

info "Testing with peer-list and peer-list-a..."
run_and_verify p2pcp-peer-mixed-a \
    -peer-list p2pcp-seeder:10090 \
    -peer-list-a p2pcp-peer.p2pcp.svc.cluster.local \
    -dst /mnt/dst -verify-on-complete -verbose -exit-complete || exit 1

info "Success"
