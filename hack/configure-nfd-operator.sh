#!/bin/bash

config_dir="$(realpath $(dirname $0)/../config)"
cd "${config_dir}"

tmp="$(mktemp -d)"

git clone --depth=1 "https://github.com/kubernetes-sigs/node-feature-discovery-operator.git" "${tmp}"
pushd "${tmp}" &>/dev/null

pushd "config" &>/dev/null
bases=$(find ./crd/bases -name "*.yaml")
sed -i 's/nfd.kubernetes.io/nfd.opni.io/g' ${bases}
rename "nfd.kubernetes.io" "nfd.opni.io" ${bases}
bases=$(find ./crd/bases -name "*.yaml")
rm -rf "${config_dir}/crd/nfd"
mkdir -p "${config_dir}"/crd/nfd
mv ${bases} "${config_dir}/crd/nfd"
popd &>/dev/null

rm -rf "${config_dir}/assets/nfd"
mv build/assets "${config_dir}/assets/nfd"

popd &>/dev/null
