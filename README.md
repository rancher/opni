# Opni

## Opni cluster setup instructions

1) Install NATS, Elasticsearch, traefik
    ```
    helm repo add bitnami https://charts.bitnami.com/bitnami
    helm repo add traefik https://helm.traefik.io/traefik
    helm repo update

   # install NATS
    helm install nats --set auth.enabled=true,auth.password=VfU6TcAl9x,replicaCount=3,maxPayload=10485760 bitnami/nats

   # (OPTIONAL) NATS loadbalancer
   helm install nats --set auth.enabled=true,auth.password=VfU6TcAl9x,replicaCount=3,client.service.type=LoadBalancer bitnami/nats

    # install Elasticsearch
    helm install elasticsearch --set global.kibanaEnabled=true bitnami/elasticsearch --render-subchart-notes

    # install traefik
    helm install traefik traefik/traefik
    ```
   * To view Kibana, `kubectl port-forward svc/elasticsearch-kibana 8080:5601` and visit http://127.0.0.1:8080
   * To view traefik dashboard:
     * `kubectl port-forward $(kubectl get pods --selector "app.kubernetes.io/name=traefik" --output=name) 9000:9000`
       * http://127.0.0.1:9000/dashboard/

   * To monitor the payload in NATS:  `kubectl run -i --rm --tty nats-box --image=synadia/nats-box --restart=Never`
        then do : `nats-sub -s "nats://nats_client:VfU6TcAl9x@nats-client.default.svc:4222" "raw_logs"`

   * To expose the endpoint of traefik,
      ```
      export ENDPOINT=`kubectl get svc traefik -o jsonpath='{.status.loadBalancer.ingress[*].hostname}'`
      ```
       and `echo $ENDPOINT`

2) Install Opni
    ```
    # install NVIDIA gpu driver
    kubectl apply -f https://raw.githubusercontent.com/NVIDIA/k8s-device-plugin/v0.6.0/nvidia-device-plugin.yml

    kubectl apply -f all-in-one.yaml
    ```
3) Or simply install everything using this command
   ```
   sh aiops_cluster_install.sh
   ```

## Contributing
We use `pre-commit` for formatting and checking import. Please refer to [installation](https://pre-commit.com/#installation) to install the pre-commit or run `pip install pre-commit`. Then you can activate it for this repo. Once it's activated, it will lint and format the code when you make a git commit. It makes changes in place. If the code is modified during the reformatting, it needs to be staged manually.

```
# Install a git commit hook to invoke automatically
pre-commit install

# Manually run against all files
pre-commit run --all-files
```
