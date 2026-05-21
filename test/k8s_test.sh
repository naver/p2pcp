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
# Download kind if not present
#######################################
ensure_kind() {
    if [[ -x "$KIND" ]]; then
        info "kind already exists, skipping download"
        return 0
    fi

    info "Downloading kind ${KIND_VERSION}..."
    local os arch
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    arch=$(uname -m)
    [[ "$arch" == "x86_64" ]] && arch="amd64"
    [[ "$arch" == "aarch64" ]] && arch="arm64"

    curl -fsSL -o "$KIND" "https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-${os}-${arch}" || return 1
    chmod +x "$KIND"
}

#######################################
# Download kubectl if not present
#######################################
ensure_kubectl() {
    if [[ -x "$KUBECTL" ]]; then
        info "kubectl already exists, skipping download"
        return 0
    fi

    info "Downloading kubectl ${KUBECTL_VERSION}..."
    local os arch
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    arch=$(uname -m)
    [[ "$arch" == "x86_64" ]] && arch="amd64"
    [[ "$arch" == "aarch64" ]] && arch="arm64"

    curl -fsSL -o "$KUBECTL" "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/${os}/${arch}/kubectl" || return 1
    chmod +x "$KUBECTL"
}

#######################################
# Run p2pcp in a pod and verify exit code
#######################################
run_and_verify() {
    local pod_name="$1"
    shift
    local p2pcp_args="$*"

    if ! "$KUBECTL" --kubeconfig "$KUBECONFIG" run -i "$pod_name" \
        --image=p2pcp-image:k8s-test \
        --image-pull-policy=IfNotPresent \
        --restart=Never \
        --namespace=p2pcp \
        --command -- bash -c "/usr/bin/p2pcp ${p2pcp_args}"; then
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
"$KUBECTL" --kubeconfig "$KUBECONFIG" wait --for=condition=ready pod -l app=p2pcp-peer -n p2pcp --timeout=600s || die "Peer pods not ready"

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
