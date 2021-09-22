#!/usr/bin/env bash

root_dir=$(dirname "$0")/..
cd "$root_dir"

etcd_version="v3.5.0"

# check what version of k8s.io/api is in use
k8s_version=$(go list -m k8s.io/api | awk '{print $2}' | sed 's/v0/v1/')
goarch="$(go env GOARCH)"
goos="$(go env GOOS)"

needs_download=false
needs_etcd_download=false
# if testbin does not exist, need to download
if [ ! -d "./testbin/bin" ]; then
  needs_download=true
  needs_etcd_download=true
fi

if [ "$needs_download" = false ]; then
  if [ -f "./testbin/bin/kube-apiserver" ] && [ -f "./testbin/bin/kube-controller-manager" ] && [ -f "./testbin/bin/kubectl" ]; then
    # if testbin exists, but binary versions are older than the k8s.io/api version, need to download
    apiserver_version=$(./testbin/bin/kube-apiserver --version | awk '{print $2}')
    kcm_version=$(./testbin/bin/kube-controller-manager --version | awk '{print $2}')
    kubectl_version=$(testbin/bin/kubectl version -o json --client | jq -r '.clientVersion.gitVersion')
    if [ "$apiserver_version" != "$k8s_version" ] || [ "$kcm_version" != "$k8s_version" ] || [ "$kubectl_version" != "$k8s_version" ]; then
      echo "kubernetes server test binaries out of date"
      needs_download=true
    fi
  else 
    echo "kubernetes server test binaries missing"
    needs_download=true
  fi
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

if [ "$needs_download" = true ]; then
  # download k8s server binaries
  temp_dir=$(mktemp -d)
  echo "downloading kubernetes ${k8s_version} server binaries"
  mkdir -p ./testbin/bin
  curl -sL "https://dl.k8s.io/${k8s_version}/kubernetes-server-${goos}-${goarch}.tar.gz" -o "${temp_dir}/kubernetes.tar.gz"
  tar -xzf "${temp_dir}/kubernetes.tar.gz" -C "${temp_dir}"

  mv "${temp_dir}/kubernetes/server/bin/kube-apiserver" "./testbin/bin/kube-apiserver"
  mv "${temp_dir}/kubernetes/server/bin/kube-controller-manager" "./testbin/bin/kube-controller-manager"
  mv "${temp_dir}/kubernetes/server/bin/kubectl" "./testbin/bin/kubectl"
  rm -rf ${temp_dir}
else
  echo "kubernetes server test binaries up to date"
fi

if [ "$needs_etcd_download" = true ]; then
  # download etcd binary
  temp_dir=$(mktemp -d)
  echo "downloading etcd ${etcd_version} binary"
  mkdir -p ./testbin/bin
  curl -sL "https://storage.googleapis.com/etcd/${etcd_version}/etcd-${etcd_version}-${goos}-${goarch}.tar.gz" -o "${temp_dir}/etcd.tar.gz"
  tar -xzf "${temp_dir}/etcd.tar.gz" -C "${temp_dir}"
  mv "${temp_dir}/etcd-${etcd_version}-${goos}-${goarch}/etcd" "./testbin/bin/etcd"
  rm -rf ${temp_dir}
else
  echo "etcd test binary up to date"
fi
