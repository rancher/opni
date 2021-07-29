# Opni = AIOps for Kubernetes + Observability Tools

Opni currently features log anomaly detection for Kubernetes.

##### What does Opni give me?
* AI generated insights on your cluster's log messages
  * **Control Plane & etcd** insights
    * Pretrained models maintained by Rancher Labs
    * Only for RKE1, RKE2, k3s clusters
  * **Workload & application** insights
    * Automatically learns what steady-state is in your workloads & applications
    * For any Kubernetes cluster  
* Every log message sent to Opni will be marked as:
  * **Normal**
  * **Suspicious** - Operators may want to investigate
  * **Anomalous** - Operators definitely should investigate  
* Open Distro for Elasticsearch + Kibana 
  * Opni dashboard to consume log insights & explore logs 
  * Ability to setup & send alerts (slack/email/etc) based on Opni log insights

![alt text](https://opni-public.s3.us-east-2.amazonaws.com/opni-inside-cluster-diagram.png)

----
## Getting started with Opni


### Full Install Opni in your Kubernetes cluster:
* Download the `opnictl` binary from the [latest release](https://github.com/rancher/opni/releases/tag/v0.1.2)
* Install Opni using `opnictl`
  ```
  opnictl install
  opnictl create demo
  ```
  * Will use your current kubeconfig context
  * Cluster Hardware requirements: 4 vCPUs, 16GB RAM
    * *2 Nvidia GPUs* required if you want the AI to learn from your workloads (recommended)
      * Next release (v0.1.3) will only require 1 GPU


Consume insights from the Opni Dashboard in Kibana. You will need to expose the Kibana service or port forward to do this.

### Demo Opni in a sandbox environment
* What you need: **an Ubuntu VM with 4 vCPUs & 16 GB RAM**
* 1-command installer that creates an RKE2 cluster with Opni installed and simulates [a failure](https://github.com/rancher/opni-docs/blob/22ed683e2b9e810b04561967d65682654350d787/quickstart_files/install_opni.sh#L72)
  ```
  curl -sfL https://raw.githubusercontent.com/rancher/opni-docs/main/quickstart_files/install_opni.sh | sh -
  ```

  * Copy the NodePort from the script output
    * View insights from the Opni Dashboard in Kibana [IPV4_ADDRESS]:[NODE_PORT]

  * Experiment with injecting your own control plane failures to see how Opni responds
    * Can refer to [these failures](https://github.com/rancher/opni-docs/blob/main/examples/fault-injection.md) and this [anomaly injection script](https://github.com/rancher/opni-docs/blob/main/quickstart_files/errors_injection.sh) as starting points

The default username and password is admin/admin You must be in the Global Tenant mode if you are not already. Navigate to `Dashboard` then `Opni Logs Dashboard`.
 
----

**Watch a demo of Opni:**

[![](https://opni-public.s3.us-east-2.amazonaws.com/opni_youtube_gh.png)](https://youtu.be/DQVBwMaO_o0)
____
#### What's next?

 * v0.1.1 (Released) allows you to view Opni's log anomaly insights **only** on a demo environment created on a VM
 * v0.1.2 (Released) allows you install Opni into your existing Kubernetes cluster and consume log insights from it
 * v0.1.3 (August 2021) - only 1 GPU required, changes to the Opni operator, log anomaly optimizations
 * v0.2.0 (Fall 2021) will introduce a custom UI, AI applied to metrics, kubernetes events, audit logs, and more! 


![alt text](https://opni-public.s3.us-east-2.amazonaws.com/Opni-user-scenarios.png)

----


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

