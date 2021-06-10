# Opni - AIOPs for Kubernetes

Opni is a collection of AIOPs tools - it currently features log anomaly detection for Kubernetes.

- [ ] **Stable v0.1 release in June 2021**
____

**Watch a demo of Opni:**

[![](https://opni-public.s3.us-east-2.amazonaws.com/opni_youtube_gh.png)](https://youtu.be/DQVBwMaO_o0)
____
## Installing
Use a 1-command install: `opnictl` - A small tool to install the opni stack on Kubernetes cluster
```
opnictl install --kubeconfig <kubeconfig path>
```

### Options
The tool can be customized to specify different passwords and versions for the stack components:
```
# opnictl install --help
NAME:
   opnictl install - install opni stack

USAGE:
   opnictl install [OPTIONS]

OPTIONS:
   --kubeconfig value               [$KUBECONFIG]
   --minio-access-key value         [$MINIO_ACCESS_KEY]
   --minio-secret-key value         [$MINIO_SECRET_KEY]
   --minio-version value           (default: "4.0.8") [$MINIO_VERSION]
   --nats-version value            (default: "2.2.1") [$NATS_VERSION]
   --nats-password value            [$NATS_PASSWORD]
   --nats-replicas value           (default: 3) [$NATS_REPLICAS]
   --nats-max-payload value        (default: 10485760) [$NATS_MAX_PAYLOAD]
   --nvidia-version value          (default: "1.0.0-beta6") [$NVIDIA_VERSION]
   --elasticsearch-user value      (default: "admin") [$ES_USER]
   --elasticsearch-password value  (default: "admin") [$ES_PASSWORD]
   --traefik-version value         (default: "v9.18.3") [$TRAEFIK_VERSION]
```
If passwords are not specified for a specific component, a random generated password will be created for it.

For deleting the stack:
```
opnictl delete --kubeconfig <kubeconfig path> --all
```
____

#### What does Opni give me?
TODO: Add architecture markdown file
* Insights into logs from your cluster's workloads, control plane & etcd
* Opni insights dashboard to inspect logs
* Ability to send alerts (slack/email/etc) when anomaly threshold is breached

Every log message sent to Opni will be marked as either normal, suspicious, or anomalous.
If a lot of logs in a short period of time are marked as suspicious or anomalous it is probably worth investigating!
The anomaly threshold is a number that can tuned depending on your volume of logs and how frequently Opni is predicting anomalies.
____
#### Prerequisites
TODO Production
* At least two GPU nodes (K80 GPU or higher)
* One node with at least 4 CPUs
* At least three additional nodes each with at least 16 GB RAM and 40 GB of disk space.

Want to try Opni out on on a lightweight cluster? (it's a lightweight opni with only control plane logs anomaly detection)
TODO
____
#### How does it work?
TODO Move this to another markdown file
Ship logs over to your Opni cluster with [Rancher Logging](https://rancher.com/docs/rancher/v2.x/en/logging/v2.5/). That's it! Opni will continuously learn the nature of your logs and will update models automatically.
____
#### Upcoming features
- Prediction feedback - give feedback for incorrect predictions so the AI adapts better to your logs
- Control plane log anomaly detection for additional Kubernetes distributions besides RKE including K3S and EKS.

____
#### Ship Logs to Opni
Fetch the Opni service endpoint by running:
```
kubectl get svc traefik -n opni-system -o jsonpath='{.status.loadBalancer.ingress[*].hostname}'
```
* Your endpoint will look something like `xyz-xxxxxxxxx.us-east-2.elb.amazonaws.com`
____

### Development
TODO Add this to its own markdown file
Building

```
# make
```

All stack manifests are under `src/` directory, any added or changed yaml will be available as part of the stack after running Make


#### macOS

For building in MACOS you need to enable cross building by using the following command:

```
# CROSS=true make
```

Then the binary should be found under ./bin/opnictl-darwin

for running the tool, you can use the following command:

```
./bin/opnictl-darwin install --kubeconfig <kubeconfig path>
```

## Contributing
TODO Add this to its own markdown file
We use `pre-commit` for formatting auto-linting and checking import. Please refer to [installation](https://pre-commit.com/#installation) to install the pre-commit or run `pip install pre-commit`. Then you can activate it for this repo. Once it's activated, it will lint and format the code when you make a git commit. It makes changes in place. If the code is modified during the reformatting, it needs to be staged manually.

```
# Install
pip install pre-commit

# Install the git commit hook to invoke automatically every time you do "git commit"
pre-commit install

# (Optional)Manually run against all files
pre-commit run --all-files
```

## License

Copyright (c) 2014-2020 [Rancher Labs, Inc.](http://rancher.com)

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

