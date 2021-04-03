### build image
```
bash build.sh
```

### Install NVIDIA GPU driver
```
kubectl apply -f https://raw.githubusercontent.com/NVIDIA/k8s-device-plugin/v0.6.0/nvidia-device-plugin.yml
```

### Start service
```
kubectl apply -f nulog-inference-service.yaml
```