#!/bin/bash

config_dir="$(realpath $(dirname $0)/../config)"
cd "${config_dir}"

tmp="$(mktemp -d)"

git clone --depth=1 "https://github.com/banzaicloud/logging-operator.git" "${tmp}"
pushd "${tmp}" &>/dev/null

cd "config"
bases=$(find ./crd/bases -name "*.yaml")
sed -i 's/logging.banzaicloud.io/logging.opni.io/g' ${bases}
rename "logging.banzaicloud.io" "logging.opni.io" ${bases}
bases=$(find ./crd/bases -name "*.yaml")
rm -rf "${config_dir}/crd/logging"
mkdir -p ${config_dir}/crd/logging
mv ${bases} "${config_dir}/crd/logging/"
popd &>/dev/null
