# Development instructions

## Prerequisites
1. Configure a private docker registry with proper certs and DNS 
2. Create a file called `tilt-options.yaml` in the root directory of this project with the following contents:
```yaml
defaultRegistry: your.registry.local
allowedContexts:
  - your-k8s-context-here
```

## Running Opni with GPU support using PCI passthrough

2. On the GPU guest instance, do not install any nvidia drivers. Ensure nvidia drivers are not loaded and there are no drivers bound to any GPU devices
```bash
(vgpu-host) $ lspci -d 10de: -k # should not list drivers in-use
(vgpu-host) $ lsmod | grep nvidia # should be empty
```
3. Install cert-manager `./install-cert-manager.sh`
4. Label nodes for tilt `./tilt-label-nodes.sh`
5. Check `config/samples/kustomization.yaml` and ensure `_v1_clusterpolicy.yaml` is included and `_v1_clusterpolicy-vgpu.yaml` is not included
6. `tilt up`
7. Wait for the GPU operator to build and install the nvidia drivers and toolkit.

## Running Opni with GPU support using vGPU

**NOTE: Ubuntu 20.04 is the ONLY supported vGPU guest OS (at the time of writing)!**

1. Configure a private docker registry with proper certs and DNS
2. When downloading the kvm driver package from the nvidia licensing portal, you should have received two separate drivers, e.g.
```
NVIDIA-Linux-x86_64-470.63.01-grid.run 
NVIDIA-Linux-x86_64-470.63-vgpu-kvm.run
```
The first runfile (-grid.run) is the *guest* driver. the second runfile (-vgpu-kvm.run) is the *host* driver. Install only the host driver on the vGPU host, and ensure no other nvidia drivers are installed via package manager. On the host, you should see similar output from nvidia-smi:
```bash
(vgpu-host) $ nvidia-smi
+-----------------------------------------------------------------------------+
| NVIDIA-SMI 470.63       Driver Version: 470.63       CUDA Version: N/A      |
|-------------------------------+----------------------+----------------------+
| GPU  Name        Persistence-M| Bus-Id        Disp.A | Volatile Uncorr. ECC |
| Fan  Temp  Perf  Pwr:Usage/Cap|         Memory-Usage | GPU-Util  Compute M. |
|                               |                      |               MIG M. |
|===============================+======================+======================|
|   0  Tesla T4            On   | 00000000:C1:00.0 Off |                    0 |
| N/A   37C    P8    16W /  70W |     82MiB / 15359MiB |      0%      Default |
|                               |                      |                  N/A |
+-------------------------------+----------------------+----------------------+
                                                                               
+-----------------------------------------------------------------------------+
| Processes:                                                                  |
|  GPU   GI   CI        PID   Type   Process name                  GPU Memory |
|        ID   ID                                                   Usage      |
|=============================================================================|
|  No running processes found                                                 |
+-----------------------------------------------------------------------------+

(vgpu-host) $ nvidia-smi vgpu
+-----------------------------------------------------------------------------+
| NVIDIA-SMI 470.63                 Driver Version: 470.63                    |
|---------------------------------+------------------------------+------------+
| GPU  Name                       | Bus-Id                       | GPU-Util   |
|      vGPU ID     Name           | VM ID     VM Name            | vGPU-Util  |
|=================================+==============================+============|
|   0  Tesla T4                   | 00000000:C1:00.0             |   0%       |
+---------------------------------+------------------------------+------------+

(vgpu-host) $ nvidia-smi -q
# ...
GPU Virtualization Mode                           
    Virtualization Mode               : Host VGPU 
    Host VGPU Mode                    : Non SR-IOV
# ...
```

Note that (at the time of writing) vGPUs are implemented using *mediated devices* instead of SR-IOV. You can check that the proper mediated devices are available as follows:
```
# Replace the pci address with your own 
(vgpu-host) $ ls /sys/bus/pci/devices/0000\:c1\:00.0/mdev_supported_types/
nvidia-222  nvidia-225  nvidia-228  nvidia-231  nvidia-234  nvidia-320    
nvidia-223  nvidia-226  nvidia-229  nvidia-232  nvidia-252  nvidia-321    
nvidia-224  nvidia-227  nvidia-230  nvidia-233  nvidia-319                
```

3. Build the *guest* driver `./vgpu-build-driver.sh /path/to/NVIDIA-Linux-###-grid.run /path/to/vgpuDriverCatalog.yaml` (the driver catalog can also be downloaded from the licensing portal)
4. Tag and push the driver to your local registry
```bash
$ docker tag nvidia-vgpu-driver:latest-ubuntu20.04 your.registry.local/nvidia-vgpu-driver:latest-ubuntu20.04 # note: keep the -ubuntu20.04 suffix
$ docker push your.registry.local/nvidia-vgpu-driver:latest-ubuntu20.04
```
5. Patch `config/samples/_v1_clusterpolicy-vgpu.yaml` as follows:
```yaml
apiVersion: opni.io/v1beta1
kind: GpuPolicyAdapter
metadata:
  name: vgpu
spec:
  images:
    driver: your.registry.local/nvidia-vgpu-driver:latest-ubuntu20.04
  vgpu:
    licenseConfigMap: licensing-config
    licenseServerKind: nls # assuming you are using NLS
```
6. Swap `_v1beta1_gpupolicyadapter.yaml` for `_v1beta1_gpupolicyadapter-vgpu.yaml` in `config/samples/kustomization.yaml`
7. Install cert-manager `./install-cert-manager.sh`
8. Label nodes for tilt `./tilt-label-nodes.sh`
9. Create the license configmap `./vgpu-create-license-configmap.sh`
10. `tilt up`
11. Wait for the GPU operator to build and install the nvidia drivers and toolkit.
12. On the vGPU *host*, you should see similar output to the following (specific values will depend on the vgpu mdev type used to launch your guest instance)
```bash
(vgpu-host) $ nvidia-smi                                                                   
Wed Sep  1 19:40:31 2021                                                       
+-----------------------------------------------------------------------------+
| NVIDIA-SMI 470.63       Driver Version: 470.63       CUDA Version: N/A      |
|-------------------------------+----------------------+----------------------+
| GPU  Name        Persistence-M| Bus-Id        Disp.A | Volatile Uncorr. ECC |
| Fan  Temp  Perf  Pwr:Usage/Cap|         Memory-Usage | GPU-Util  Compute M. |
|                               |                      |               MIG M. |
|===============================+======================+======================|
|   0  Tesla T4            On   | 00000000:C1:00.0 Off |                    0 |
| N/A   37C    P8    16W /  70W |   3858MiB / 15359MiB |      0%      Default |
|                               |                      |                  N/A |
+-------------------------------+----------------------+----------------------+
                                                                               
+-----------------------------------------------------------------------------+
| Processes:                                                                  |
|  GPU   GI   CI        PID   Type   Process name                  GPU Memory |
|        ID   ID                                                   Usage      |
|=============================================================================|
|    0   N/A  N/A   2693296    C+G   vgpu                             3776MiB |
+-----------------------------------------------------------------------------+

(vgpu-host) $ nvidia-smi vgpu                                                              
Wed Sep  1 19:40:33 2021                                                       
+-----------------------------------------------------------------------------+
| NVIDIA-SMI 470.63                 Driver Version: 470.63                    |
|---------------------------------+------------------------------+------------+
| GPU  Name                       | Bus-Id                       | GPU-Util   |
|      vGPU ID     Name           | VM ID     VM Name            | vGPU-Util  |
|=================================+==============================+============|
|   0  Tesla T4                   | 00000000:C1:00.0             |   0%       |
|      3251634198  GRID T4-4C     | a84d...  instance-000000d5   |      0%    |
+---------------------------------+------------------------------+------------+
```
