#!/bin/bash

cd "$(realpath $(dirname $0)/..)"

kubectl kustomize ./config/default -o ./staging/staging_autogen.yaml