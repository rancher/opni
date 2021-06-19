# Opni - AIOPs for Kubernetes

Opni is a collection of AIOPs tools - it currently features log anomaly detection for Kubernetes.

- [ ] **Stable v0.1 release in June 2021**
____

**Watch a demo of Opni:**

[![](https://opni-public.s3.us-east-2.amazonaws.com/opni_youtube_gh.png)](https://youtu.be/DQVBwMaO_o0)
____

#### What does Opni give me?
* Insights into logs from your cluster's workloads, control plane & etcd
* Opni insights dashboard to inspect logs
* Ability to send alerts (slack/email/etc) when anomaly threshold is breached

Every log message sent to Opni will be marked as either normal, suspicious, or anomalous.
If a lot of logs in a short period of time are marked as suspicious or anomalous it is probably worth investigating!
The anomaly threshold is a number that can tuned depending on your volume of logs and how frequently Opni is predicting anomalies.
____
#### Virtual Machine
* Virtual Machine Instance with:
   * 4 CPUs
   * 16GB of RAM
   * 32GB of disk space

____
#### How does it work?
 When Opni is installed into your cluster, it will also install Rancher logging into the cluster.
 Rancher logging will aggregate control plane log messages within the cluster and send them over to Opni.
 Opni will continuously learn the nature of your logs and will update its models automatically.
____

#### Documentation
-------------

Please see [the official docs site](https://opni.io/) for complete documentation.

**Quick-Start - Install Script**
--------------

To install Opni, within your virtual machine instance run:

```
curl -sfL https://raw.githubusercontent.com/rancher/opni-docs/demo/quickstart_files/install_opni.sh | sh -
```

A kubeconfig file is written to `/etc/rancher/rke2/rke2.yaml` and the service is automatically started or restarted.
The install script will first install RKE2 and additional utilities such as `kubectl`.
Once the RKE2 cluster has been setup, the script then installs Opni and Rancher logging onto the cluster.
Finally, once that has been completed, it will inject an anomaly.

____
#### Ship Logs to Opni
Fetch the Opni service endpoint by running:
```
kubectl get svc traefik -n opni-system -o jsonpath='{.status.loadBalancer.ingress[*].hostname}'
```
* Your endpoint will look something like `xyz-xxxxxxxxx.us-east-2.elb.amazonaws.com`
____
#### Upcoming features
- Prediction feedback - give feedback for incorrect predictions so the AI adapts better to your logs
- Control plane log anomaly detection for additional Kubernetes distributions besides RKE including K3S and EKS.

## Contributing
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