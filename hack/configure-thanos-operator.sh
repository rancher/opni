#!/bin/bash

config_dir="$(realpath $(dirname $0)/../config)"
cd "${config_dir}"

tmp="$(mktemp -d)"

git clone --depth=1 "https://github.com/banzaicloud/thanos-operator.git" "${tmp}"
pushd "${tmp}" &>/dev/null

cd "config"
bases=$(find ./crd/bases -name "*.yaml")
sed -i 's/monitoring.banzaicloud.io/thanos.opni.io/g' ${bases}
rename "monitoring.banzaicloud.io" "thanos.opni.io" ${bases}
bases=$(find ./crd/bases -name "*.yaml")
rm -rf "${config_dir}/crd/thanos"
mkdir -p ${config_dir}/crd/thanos
mv ${bases} "${config_dir}/crd/thanos/"
popd &>/dev/null
