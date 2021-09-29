#!/bin/sh

set -e

info() {
    echo "[INFO] " "$@"
}

fatal() {
    echo "[ERROR] " "$@" >&2
    exit 1
}

verify_kubectl() {
    cmd="$(command -v "kubectl")"
    if [ -z "${cmd}" ]; then
        return 1
    fi
    if [ ! -x "${cmd}" ]; then
        return 1
    fi

    return 0
}

verify_kubectl || fatal "Installation script requires kubectl to be in the path"

info "Installing Opni"
kubectl  apply -f https://raw.githubusercontent.com/rancher/opni/main/deploy/manifests/00_crds.yaml > /dev/null 2>&1
kubectl  apply -f https://raw.githubusercontent.com/rancher/opni/main/deploy/manifests/01_rbac.yaml > /dev/null 2>&1
kubectl  apply -f https://raw.githubusercontent.com/rancher/opni/main/deploy/manifests/10_operator.yaml > /dev/null 2>&1
kubectl  wait --timeout=300s --for=condition=available deploy/opni-controller-manager -n opni-system
kubectl apply -f https://raw.githubusercontent.com/rancher/opni/main/deploy/manifests/20_cluster.yaml > /dev/null 2>&1