#!/bin/bash

token_file=$(find /etc/nvidia/ClientConfigToken -type f | head -1)

kubectl -n opni-system create configmap licensing-config \
  --from-file=/etc/nvidia/gridd.conf \
  --from-file=client_configuration_token.tok=$token_file
