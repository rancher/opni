#!/usr/bin/env bash

root_dir=$(dirname "$0")/..
cd "$root_dir" || exit 1

etcd_version="v3.5.1"
cortex_version="v1.11.0"
goarch="$(go env GOARCH)"
goos="$(go env GOOS)"

needs_etcd_download=false
needs_cortex_download=false
# if testbin does not exist, need to download
if [ ! -d "./testbin/bin" ]; then
  needs_etcd_download=true
  needs_cortex_download=true
fi

if [ "$needs_etcd_download" = false ]; then
  if [ -f "./testbin/bin/etcd" ]; then
    cur_etcd_version="v$(testbin/bin/etcd --version | head -1 | awk '{print $3}')"
    if [ "$cur_etcd_version" != "$etcd_version" ]; then
      echo "etcd test binary out of date"
      needs_etcd_download=true
    fi
  else
    echo "etcd test binary missing"
    needs_etcd_download=true
  fi
fi

if [ "$needs_etcd_download" = true ]; then
  # download etcd binary
  temp_dir=$(mktemp -d)
  echo "downloading etcd ${etcd_version} binary"
  mkdir -p ./testbin/bin
  curl -sL "https://storage.googleapis.com/etcd/${etcd_version}/etcd-${etcd_version}-${goos}-${goarch}.tar.gz" -o "${temp_dir}/etcd.tar.gz"
  tar -xzf "${temp_dir}/etcd.tar.gz" -C "${temp_dir}"
  mv "${temp_dir}/etcd-${etcd_version}-${goos}-${goarch}/etcd" "./testbin/bin/etcd"
  rm -rf "${temp_dir}"
else
  echo "etcd test binary up to date"
fi

if [ "$needs_cortex_download" = false ]; then
  if [ -f "./testbin/bin/cortex" ]; then
    cur_cortex_version="v$(testbin/bin/cortex --version | head -1 | awk '{print $3}')"
    if [ "$cur_cortex_version" != "$cortex_version" ]; then
      echo "cortex test binary out of date"
      needs_cortex_download=true
    fi
  else
    echo "cortex test binary missing"
    needs_cortex_download=true
  fi
fi

if [ "$needs_cortex_download" = true ]; then
  # download cortex binary
  temp_dir=$(mktemp -d)
  echo "downloading cortex ${cortex_version} binary"
  mkdir -p ./testbin/bin
  curl -sL "https://github.com/cortexproject/cortex/releases/download/${cortex_version}/cortex-${goos}-${goarch}" -o "${temp_dir}/cortex"
  chmod +x "${temp_dir}/cortex"
  mv "${temp_dir}/cortex" "./testbin/bin/cortex"
  rm -rf "${temp_dir}"
else
  echo "cortex test binary up to date"
fi
