module github.com/rancher/opni

go 1.16

require (
	emperror.dev/errors v0.8.0
	github.com/AlecAivazis/survey/v2 v2.2.16
	github.com/Microsoft/go-winio v0.5.0 // indirect
	github.com/Microsoft/hcsshim v0.8.20 // indirect
	github.com/banzaicloud/logging-operator v1.12.4-alpine-2
	github.com/banzaicloud/logging-operator/pkg/sdk v0.7.7
	github.com/banzaicloud/operator-tools v0.25.0
	github.com/elastic/go-elasticsearch/v7 v7.12.0
	github.com/go-logr/logr v0.4.0
	github.com/k3s-io/helm-controller v0.11.2
	github.com/kralicky/highlander v0.0.0-20210804214334-9cfe339efd8a
	github.com/kralicky/kmatch v0.0.0-20210817213559-44a41c23a9fd
	github.com/longhorn/upgrade-responder v0.1.2-0.20210521005936-d72e5ddbc541
	github.com/nats-io/nkeys v0.3.0
	github.com/onsi/ginkgo v1.16.5-0.20210808002315-bc059d745883
	github.com/onsi/gomega v1.15.0
	github.com/opencontainers/image-spec v1.0.2-0.20190823105129-775207bd45b6 // indirect
	github.com/opencontainers/runc v1.0.1 // indirect
	github.com/phayes/freeport v0.0.0-20180830031419-95f893ade6f2
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.50.0
	github.com/rancher/k3d/v4 v4.4.7
	github.com/spf13/cobra v1.2.1
	github.com/ttacon/chalk v0.0.0-20160626202418-22c06c80ed31
	github.com/vbauerster/mpb/v7 v7.1.3
	go.uber.org/atomic v1.9.0
	go.uber.org/zap v1.19.0
	golang.org/x/mod v0.5.0
	golang.org/x/tools v0.1.5 // indirect
	k8s.io/api v0.22.0
	k8s.io/apiextensions-apiserver v0.22.0
	k8s.io/apimachinery v0.22.0
	k8s.io/client-go v0.22.0
	k8s.io/utils v0.0.0-20210802155522-efc7438f0176
	sigs.k8s.io/controller-runtime v0.9.6
)

replace github.com/banzaicloud/logging-operator/pkg/sdk => github.com/banzaicloud/logging-operator/pkg/sdk v0.7.7
