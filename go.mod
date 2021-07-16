module github.com/rancher/opni

go 1.16

require (
	github.com/AlecAivazis/survey/v2 v2.2.12
	github.com/banzaicloud/logging-operator v0.0.0-20210712114307-bf2e09ce9302
	github.com/banzaicloud/logging-operator/pkg/sdk v0.7.5
	github.com/banzaicloud/operator-tools v0.23.0
	github.com/containers/image/v5 v5.12.0
	github.com/elastic/go-elasticsearch/v7 v7.13.1
	github.com/go-logr/logr v0.4.0
	github.com/k3s-io/helm-controller v0.10.0
	github.com/longhorn/upgrade-responder v0.1.2-0.20210521005936-d72e5ddbc541
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.13.0
	github.com/phayes/freeport v0.0.0-20180830031419-95f893ade6f2
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.43.0
	github.com/rancher/k3d/v4 v4.4.5
	github.com/spf13/cobra v1.1.3
	github.com/ttacon/chalk v0.0.0-20160626202418-22c06c80ed31
	github.com/vbauerster/mpb/v7 v7.0.2
	go.uber.org/atomic v1.8.0
	go.uber.org/zap v1.17.0
	golang.org/x/mod v0.4.2
	golang.org/x/tools v0.1.3 // indirect
	k8s.io/api v0.21.1
	k8s.io/apiextensions-apiserver v0.21.1
	k8s.io/apimachinery v0.21.1
	k8s.io/client-go v0.21.1
	k8s.io/utils v0.0.0-20210527160623-6fdb442a123b
	sigs.k8s.io/controller-runtime v0.9.0
)

replace github.com/banzaicloud/logging-operator/pkg/sdk => github.com/banzaicloud/logging-operator/pkg/sdk v0.7.5
