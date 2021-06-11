#!/bin/bash

set -e 

HERE="$(dirname $(realpath $0))"
TESTBIN="${HERE}/../../testbin/bin"
ARCH=$(go env GOARCH)
K3D_VERSION="v4.4.4"
TEST_CLUSTER_NAME="opni-e2e-test"

if ! command -v k3d &>/dev/null; then
  curl -sL "https://github.com/rancher/k3d/releases/download/${K3D_VERSION}/k3d-linux-${ARCH}" > ${TESTBIN}/k3d
fi

export PATH="${TESTBIN}${PATH:+:${PATH}}"

for cluster in $(k3d cluster list | tail -n +2 | awk '{print $1}'); do 
  if [[ "${cluster}" == "${TEST_CLUSTER_NAME}" ]]; then
    echo "Test cluster already exists, recreating..."
    k3d cluster delete ${cluster}
    break
  fi
done

k3d cluster create \
  --registry-create \
  --servers 1 \
  --agents 1 \
  --timeout 30s \
  --kubeconfig-update-default=false \
  --kubeconfig-switch-context=false \
  ${TEST_CLUSTER_NAME} "$@"

export KUBECONFIG=$(k3d kubeconfig get ${TEST_CLUSTER_NAME})