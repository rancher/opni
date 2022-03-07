module github.com/rancher/opni

go 1.18

require (
	emperror.dev/errors v0.8.1
	github.com/AlecAivazis/survey/v2 v2.3.2
	github.com/NVIDIA/gpu-operator v1.8.1
	github.com/banzaicloud/logging-operator v0.0.0-20220225205714-b06e7ad17676
	github.com/banzaicloud/logging-operator/pkg/sdk v0.7.19
	github.com/banzaicloud/operator-tools v0.28.2
	github.com/go-logr/logr v1.2.2
	github.com/go-test/deep v1.0.7
	github.com/hashicorp/go-version v1.3.0
	github.com/imdario/mergo v0.3.12
	github.com/jarcoal/httpmock v1.0.8
	github.com/k3s-io/helm-controller v0.11.2
	github.com/kralicky/highlander v0.0.0-20210804214334-9cfe339efd8a
	github.com/kralicky/kmatch v0.0.0-20210910033132-e5a80a7a45e6
	github.com/kubernetes-sigs/node-feature-discovery-operator v0.2.1-0.20210826163723-568b36491208
	github.com/longhorn/upgrade-responder v0.1.2-0.20210521005936-d72e5ddbc541
	github.com/mattn/go-isatty v0.0.14
	github.com/nats-io/nkeys v0.3.0
	github.com/onsi/ginkgo v1.16.6-0.20211118180735-4e1925ba4c95
	github.com/onsi/gomega v1.18.1
	github.com/opensearch-project/opensearch-go v1.0.0
	github.com/phayes/freeport v0.0.0-20220201140144-74d24b5ae9f5
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.52.0
	github.com/rancher/opni-monitoring v0.0.0-20220313224055-57eed34456f2
	github.com/spf13/cobra v1.3.0
	github.com/spf13/pflag v1.0.5
	github.com/ttacon/chalk v0.0.0-20160626202418-22c06c80ed31
	github.com/vbauerster/mpb/v7 v7.1.3
	go.uber.org/atomic v1.9.0
	go.uber.org/zap v1.21.0
	golang.org/x/crypto v0.0.0-20220214200702-86341886e292
	golang.org/x/mod v0.6.0-dev.0.20211013180041-c96bc1413d57
	k8s.io/api v0.23.4
	k8s.io/apiextensions-apiserver v0.23.4
	k8s.io/apimachinery v0.23.4
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/component-base v0.23.4
	k8s.io/utils v0.0.0-20220210201930-3a6ce19ff2f9
	opensearch.opster.io v0.0.0-00010101000000-000000000000
	sigs.k8s.io/controller-runtime v0.11.1
)

require (
	cloud.google.com/go v0.99.0 // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.11.18 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.13 // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/VividCortex/ewma v1.2.0 // indirect
	github.com/acarl005/stripansi v0.0.0-20180116102854-5a71ef0e047d // indirect
	github.com/alecthomas/jsonschema v0.0.0-20220216202328-9eeeec9d044b // indirect
	github.com/andreyvit/diff v0.0.0-20170406064948-c7f18ee00883 // indirect
	github.com/andybalholm/brotli v1.0.4 // indirect
	github.com/banzaicloud/k8s-objectmatcher v1.8.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/briandowns/spinner v1.12.0 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/cppforlife/go-patch v0.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.0-20210816181553-5444fa50b93d // indirect
	github.com/evanphx/json-patch v5.6.0+incompatible // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/form3tech-oss/jwt-go v3.2.3+incompatible // indirect
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/go-logr/zapr v1.2.0 // indirect
	github.com/goccy/go-json v0.9.4 // indirect
	github.com/gofiber/fiber/v2 v2.28.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-cmp v0.5.7 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/googleapis/gnostic v0.5.5 // indirect
	github.com/hashicorp/go-hclog v1.1.0 // indirect
	github.com/hashicorp/go-plugin v1.4.3 // indirect
	github.com/hashicorp/yamux v0.0.0-20180604194846-3520598351bb // indirect
	github.com/iancoleman/orderedmap v0.2.0 // indirect
	github.com/iancoleman/strcase v0.2.0 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/jwalton/go-supportscolor v1.1.0 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/klauspost/compress v1.14.1 // indirect
	github.com/lestrrat-go/backoff/v2 v2.0.8 // indirect
	github.com/lestrrat-go/blackmagic v1.0.0 // indirect
	github.com/lestrrat-go/httpcc v1.0.0 // indirect
	github.com/lestrrat-go/iter v1.0.1 // indirect
	github.com/lestrrat-go/jwx v1.2.20 // indirect
	github.com/lestrrat-go/option v1.0.0 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/mgutz/ansi v0.0.0-20170206155736-9520e82c474b // indirect
	github.com/mitchellh/go-testing-interface v1.0.0 // indirect
	github.com/mitchellh/hashstructure v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.4.3 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/oklog/run v1.0.0 // indirect
	github.com/onsi/ginkgo/v2 v2.1.3 // indirect
	github.com/openshift/api v3.9.0+incompatible // indirect
	github.com/openshift/client-go v0.0.0-20210112165513-ebc401615f47 // indirect
	github.com/openshift/custom-resource-status v0.0.0-20210221154447-420d9ecf2a00 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.11.0 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.32.1 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/sergi/go-diff v1.1.0 // indirect
	github.com/spf13/cast v1.4.1 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.33.0 // indirect
	github.com/valyala/tcplisten v1.0.0 // indirect
	github.com/wayneashleyberry/terminal-dimensions v1.0.0 // indirect
	go.uber.org/multierr v1.7.0 // indirect
	golang.org/x/net v0.0.0-20220114011407-0dd24b26b47d // indirect
	golang.org/x/oauth2 v0.0.0-20211104180415-d3ed0bb246c8 // indirect
	golang.org/x/sys v0.0.0-20220128215802-99c3d69c2c27 // indirect
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211 // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/time v0.0.0-20211116232009-f0f3c7e86c11 // indirect
	gomodules.xyz/jsonpatch/v2 v2.2.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20220304144024-325a89244dc8 // indirect
	google.golang.org/grpc v1.44.0 // indirect
	google.golang.org/protobuf v1.27.1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
	k8s.io/klog v1.0.0 // indirect
	k8s.io/klog/v2 v2.40.1 // indirect
	k8s.io/kube-openapi v0.0.0-20220114203427-a0453230fd26 // indirect
	k8s.io/kubectl v0.23.4 // indirect
	sigs.k8s.io/json v0.0.0-20211208200746-9f7c6b3444d2 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.1 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace (
	github.com/NVIDIA/gpu-operator => github.com/kralicky/gpu-operator v1.8.1-0.20211112183255-72529edf38be

	github.com/banzaicloud/logging-operator/pkg/sdk => github.com/banzaicloud/logging-operator/pkg/sdk v0.0.0-20220225205714-b06e7ad17676
	// github.com/banzaicloud/logging-operator/pkg/sdk => github.com/banzaicloud/logging-operator/pkg/sdk v0.7.7
	github.com/openshift/api => github.com/openshift/api v0.0.0-20210216211028-bb81baaf35cd

	// Because of a dependency chain to Coretx
	k8s.io/client-go => k8s.io/client-go v0.23.4
	opensearch.opster.io => github.com/Opster/opensearch-k8s-operator/opensearch-operator v0.0.0-20220221125137-07e7c4a0839e
)
