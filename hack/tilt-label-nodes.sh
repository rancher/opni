#!/bin/bash

if [ $# -ne 1 ] || [ "$1" = "-h" ] || [ "$1" = "--help" ]; then
  echo "Usage: $0 registry_ip[:port]" 
  exit 1
fi

# Annotate nodes with registry info for Tilt to auto-detect
echo "Annotating nodes with registry info..."
DONE=""
nodes=$(kubectl get nodes -o go-template --template='{{range .items}}{{printf "%s\n" .metadata.name}}{{end}}')
for node in $nodes; do
  kubectl annotate node "${node}" --overwrite \
          tilt.dev/registry=$1 \
          tilt.dev/registry-from-cluster=$1
done