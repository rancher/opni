#!/bin/bash

kubectl get ns gpu-operator-resources &> /dev/null || kubectl create ns gpu-operator-resources

token_file=$(find /etc/nvidia/ClientConfigToken -type f | head -1)

kubectl -n gpu-operator-resources create configmap licensing-config \
  --from-file=/etc/nvidia/gridd.conf \
  --from-file=client_configuration_token.tok=$token_file
