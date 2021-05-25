module github.com/rancher/opnictl

go 1.13

replace (
	github.com/kubernetes-sigs/cri-tools => github.com/k3s-io/cri-tools v1.20.0-k3s1
	github.com/rancher/wrangler => github.com/rancher/wrangler v0.6.1
	k8s.io/api => github.com/k3s-io/kubernetes/staging/src/k8s.io/api v1.20.2-k3s1
	k8s.io/apiextensions-apiserver => github.com/k3s-io/kubernetes/staging/src/k8s.io/apiextensions-apiserver v1.20.2-k3s1
	k8s.io/apimachinery => github.com/k3s-io/kubernetes/staging/src/k8s.io/apimachinery v1.20.2-k3s1
	k8s.io/apiserver => github.com/k3s-io/kubernetes/staging/src/k8s.io/apiserver v1.20.2-k3s1
	k8s.io/cli-runtime => github.com/k3s-io/kubernetes/staging/src/k8s.io/cli-runtime v1.20.2-k3s1
	k8s.io/client-go => github.com/k3s-io/kubernetes/staging/src/k8s.io/client-go v1.20.2-k3s1
	k8s.io/cloud-provider => github.com/k3s-io/kubernetes/staging/src/k8s.io/cloud-provider v1.20.2-k3s1
	k8s.io/cluster-bootstrap => github.com/k3s-io/kubernetes/staging/src/k8s.io/cluster-bootstrap v1.20.2-k3s1
	k8s.io/code-generator => github.com/k3s-io/kubernetes/staging/src/k8s.io/code-generator v1.20.2-k3s1
	k8s.io/component-base => github.com/k3s-io/kubernetes/staging/src/k8s.io/component-base v1.20.2-k3s1
	k8s.io/component-helpers => github.com/k3s-io/kubernetes/staging/src/k8s.io/component-helpers v1.20.2-k3s1
	k8s.io/controller-manager => github.com/k3s-io/kubernetes/staging/src/k8s.io/controller-manager v1.20.2-k3s1
	k8s.io/cri-api => github.com/k3s-io/kubernetes/staging/src/k8s.io/cri-api v1.20.2-k3s1
	k8s.io/csi-translation-lib => github.com/k3s-io/kubernetes/staging/src/k8s.io/csi-translation-lib v1.20.2-k3s1
	k8s.io/kube-aggregator => github.com/k3s-io/kubernetes/staging/src/k8s.io/kube-aggregator v1.20.2-k3s1
	k8s.io/kube-controller-manager => github.com/k3s-io/kubernetes/staging/src/k8s.io/kube-controller-manager v1.20.2-k3s1
	k8s.io/kube-proxy => github.com/k3s-io/kubernetes/staging/src/k8s.io/kube-proxy v1.20.2-k3s1
	k8s.io/kube-scheduler => github.com/k3s-io/kubernetes/staging/src/k8s.io/kube-scheduler v1.20.2-k3s1
	k8s.io/kubectl => github.com/k3s-io/kubernetes/staging/src/k8s.io/kubectl v1.20.2-k3s1
	k8s.io/kubelet => github.com/k3s-io/kubernetes/staging/src/k8s.io/kubelet v1.20.2-k3s1
	k8s.io/kubernetes => github.com/k3s-io/kubernetes v1.20.2-k3s1
	k8s.io/legacy-cloud-providers => github.com/k3s-io/kubernetes/staging/src/k8s.io/legacy-cloud-providers v1.20.2-k3s1
	k8s.io/metrics => github.com/k3s-io/kubernetes/staging/src/k8s.io/metrics v1.20.2-k3s1
	k8s.io/mount-utils => github.com/k3s-io/kubernetes/staging/src/k8s.io/mount-utils v1.20.2-k3s1
	k8s.io/node-api => github.com/k3s-io/kubernetes/staging/src/k8s.io/node-api v1.20.2-k3s1
	k8s.io/sample-apiserver => github.com/k3s-io/kubernetes/staging/src/k8s.io/sample-apiserver v1.20.2-k3s1
	k8s.io/sample-cli-plugin => github.com/k3s-io/kubernetes/staging/src/k8s.io/sample-cli-plugin v1.20.2-k3s1
	k8s.io/sample-controller => github.com/k3s-io/kubernetes/staging/src/k8s.io/sample-controller v1.20.2-k3s1
)

require (
	github.com/go-bindata/go-bindata v3.1.2+incompatible
	github.com/k3s-io/helm-controller v0.8.4
	github.com/mitchellh/go-homedir v1.1.0
	github.com/rancher/wrangler v0.8.0
	github.com/rancher/wrangler-api v0.6.0
	github.com/sirupsen/logrus v1.6.0
	github.com/urfave/cli v1.22.2
	k8s.io/api v0.18.0
	k8s.io/apimachinery v0.18.8
	k8s.io/client-go v0.18.8
)
