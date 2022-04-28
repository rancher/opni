#!/bin/bash

config_dir="$(realpath $(dirname $0)/../config)"
cd "${config_dir}"

tmp="$(mktemp -d)"

git clone --depth=1 "https://github.com/grafana-operator/grafana-operator" "${tmp}"
pushd "${tmp}" &>/dev/null

cd "config"
bases=$(find ./crd/bases -name "*.yaml")
sed -i 's/integreatly.org/grafana.opni.io/g' ${bases}
rename "integreatly.org" "grafana.opni.io" ${bases}
bases=$(find ./crd/bases -name "*.yaml")
rm -rf "${config_dir}/crd/grafana"
mkdir -p ${config_dir}/crd/grafana
mv ${bases} "${config_dir}/crd/grafana/"
popd &>/dev/null
