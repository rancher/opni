# Opni = AIOps for Kubernetes + Observability Tools

Opni currently features log anomaly detection for Kubernetes.

##### What does Opni give me?
* AI generated insights on your cluster's log messages
  * **Control Plane & etcd** insights
    * Pretrained models maintained by Rancher Labs
* Every log message sent to Opni will be marked as:
  * **Normal**
  * **Suspicious** - Operators may want to investigate
  * **Anomalous** - Operators definitely should investigate  
* Opensearch + Opensearch Dashboards
  * Opni dashboard to consume log insights & explore logs 

![alt text](https://opni-public.s3.us-east-2.amazonaws.com/opni-inside-cluster-diagram.png)

----

### Deprecation Notice
  - GPU Learning is temporarily disabled in the v0.4.0 release as Opni moves to a multicluster architecture.  This will be returning in a future release
  - The v1beta1 API has been deprecated in this release.  Please migrate to v1beta2.
  - The UI and Insights services, which were experimental, have been removed

## Getting started with Opni

### Full Install Opni in your Kubernetes cluster:

#### Manifests install (recommended):
Prerequisites:
  * Cert manager installed.  This can be installed with the following command:
    ```bash
    kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.7.2/cert-manager.yaml
    ```
  * Opni Gateway installed - see the [Main Cluster docs](https://rancher.github.io/opni-monitoring/installation/#setting-up-the-main-cluster) for Opni Monitoring

Installation:
  1) All clusters (both the main cluster and clusters to collect logs from) the manifests in [deploy/manifests](https://github.com/rancher/opni/tree/main/deploy/manifests) in order from 00 - 10.
  1) Deploy an Opensearch cluster e.g (node this cluster will need to be exposed via a LoadBalancer or Ingress to allow logs to be indexed)
      ```yaml
      apiVersion: opensearch.opster.io/v1
      kind: OpenSearchCluster
      metadata:
        name: opni
        namespace: opni-cluster-system
      spec:
        # Add fields here
        general:
          httpPort: 9200
          vendor: opensearch
          version: 1.2.3
          serviceName: os-svc
          setVMMaxMapCount: true
        confMgmt:
          autoScaler: false
          monitoring: false
        dashboards:
          enable: true
          version: 1.2.0
          replicas: 1
        nodePools:
        - component: master
          replicas: 3
          diskSize: 32
          resources:
            requests:
              cpu: 500m
              memory: 1Gi
            limits:
              memory: 1Gi
          roles:
          - master
          persistence:
            emptyDir: {}
        - component: nodes
          replicas: 2
          diskSize: 32
          resources:
            requests:
              cpu: 500m
              memory: 2Gi
            limits:
              memory: 2Gi
          jvm: "-Xmx1G -Xms1G"
          roles:
          - data
          persistence:
            emptyDir: {}
      ```
  1) Bind Opni to the Opensearch cluster:
      ```yaml
      apiVersion: opni.io/v1beta2
      kind: MulticlusterRoleBinding
      metadata:
        name: opni-logging
        namespace: opni-cluster-system
      spec:
        opensearch:
          name: opni
          namespace: opni-cluster-system
        opensearchExternalURL: https://external.opensearch.url
      ```
  1) Deploy the Opni pretrained Kubernetes model
      ```yaml
      apiVersion: opni.io/v1beta2
      kind: PretrainedModel
      metadata:
        name: control-plane
        namespace: opni-cluster-system
      spec:
        source:
          http:
            url: "https://opni-public.s3.us-east-2.amazonaws.com/pretrain-models/control-plane-model-v0.4.0.zip"
        hyperparameters:
          modelThreshold: "0.6"
          minLogTokens: 1
          isControlPlane: "true"
      ```
  1) Deploy Opni AI services
      ```yaml
      apiVersion: opni.io/v1beta2
      kind: OpniCluster
      metadata:
        name: demo
        namespace: opni-cluster-system
      spec:
        version: v0.4.0
        deployLogCollector: false
        services:
          gpuController:
            enabled: false
          inference:
            pretrainedModels:
            - name: control-plane
        opensearch:
          externalOpensearch:
            name: opni
            namespace: opni-cluster-system
          enableLogIndexManagement: false
        s3:
          internal: {}
        nats:
          authMethod: nkey
      ```
  1) Add additional Logging clusters from the Opni Gateway UI


Consume insights from the Opni Dashboard in Opensearch Dashboards. You will need to expose the Dashboards service or port forward to do this.
 
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

