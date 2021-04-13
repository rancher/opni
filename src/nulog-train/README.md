### Build image
```
sh build.sh
```

### Install NVIDIA GPU driver (required)
```
kubectl apply -f https://raw.githubusercontent.com/NVIDIA/k8s-device-plugin/v0.6.0/nvidia-device-plugin.yml
```
