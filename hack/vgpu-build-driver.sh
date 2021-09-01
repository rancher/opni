#!/bin/bash
set -e

if [ "$#" -ne 2 ]; then
  echo "Usage: build-vgpu-driver.sh </path/to/vgpu-guest-driver.run> </path/to/vgpuDriverCatalog.yaml>"
  exit 1
fi

version=$(echo $1 | awk 'match($0,/[0-9\.]{5,}/,m) {print m[0]}')
rm -rf /tmp/driver
git clone https://gitlab.com/nvidia/container-images/driver /tmp/driver
pushd /tmp/driver
cd ubuntu20.04
cp $1 $2 drivers
docker build \
  --no-cache \
  --build-arg=DRIVER_TYPE=vgpu \
  --build-arg=VGPU_LICENSE_SERVER_TYPE=NLS \
  --build-arg=DRIVER_BRANCH=$(echo $version | cut -d. -f1) \
  --build-arg=DRIVER_VERSION=$version \
  -t nvidia-vgpu-driver:latest-ubuntu20.04 .
popd
rm -rf /tmp/driver
