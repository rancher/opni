#!/usr/bin/env bash

# This script is based on https://github.com/tilt-dev/k3d-local-registry/blob/master/k3d-with-registry.sh

# Starts a k3s cluster (via k3d) with local image registry enabled,
# and with nodes annotated such that Tilt (https://tilt.dev/) can
# auto-detect the registry.

set -o errexit

# desired cluster name (default is "k3s-default")
CLUSTER_NAME="${CLUSTER_NAME:-k3s-tilt-opni}"

KUBECONFIG= k3d cluster create \
  --registry-create \
  --servers 1 \
  --agents 1 \
  --timeout 30s \
  --volume /etc/os-release:/etc/os-release \
  ${CLUSTER_NAME} "$@"

# default name/port
# TODO(maia): support other names/ports
reg_name='registry.local'
reg_port='5000'

# Annotate nodes with registry info for Tilt to auto-detect
echo "Annotating nodes with registry info..."
DONE=""
nodes=$(kubectl --context=k3d-${CLUSTER_NAME} get nodes -o go-template --template='{{range .items}}{{printf "%s\n" .metadata.name}}{{end}}')
for node in $nodes; do
  kubectl --context=k3d-${CLUSTER_NAME} annotate node "${node}" \
          tilt.dev/registry=localhost:${reg_port} \
          tilt.dev/registry-from-cluster=${reg_name}:${reg_port}
done
