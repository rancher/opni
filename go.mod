module github.com/rancher/opni

go 1.21

require (
	emperror.dev/errors v0.8.1
	github.com/99designs/keyring v1.2.2
	github.com/AlecAivazis/survey/v2 v2.3.6
	github.com/Masterminds/semver v1.5.0
	github.com/Masterminds/sprig/v3 v3.2.3
	github.com/alecthomas/jsonschema v0.0.0-20220216202328-9eeeec9d044b
	github.com/alecthomas/kingpin/v2 v2.3.2
	github.com/alitto/pond v1.8.3
	github.com/andybalholm/brotli v1.0.5
	github.com/araddon/dateparse v0.0.0-20210429162001-6b43995a97de
	github.com/aws/aws-sdk-go v1.44.245
	github.com/bmatcuk/doublestar v1.3.4
	github.com/bmatcuk/doublestar/v4 v4.6.0
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/cert-manager/cert-manager v1.10.2
	github.com/charmbracelet/bubbles v0.15.0
	github.com/charmbracelet/bubbletea v0.23.2
	github.com/charmbracelet/lipgloss v0.7.1
	github.com/cisco-open/k8s-objectmatcher v1.9.0
	github.com/cisco-open/operator-tools v0.30.0
	github.com/cortexproject/cortex v0.0.0-00010101000000-000000000000
	github.com/dbason/featureflags v0.0.0-20220803034705-b6242a8d72b2
	github.com/dterei/gotsc v0.0.0-20160722215413-e78f872945c6
	github.com/gabstv/go-bsdiff v1.0.5
	github.com/gin-contrib/pprof v1.4.0
	github.com/gin-gonic/gin v1.9.1
	github.com/go-kit/log v0.2.1
	github.com/go-logr/logr v1.2.4
	github.com/go-logr/zapr v1.2.4
	github.com/go-openapi/strfmt v0.21.7
	github.com/go-playground/validator/v10 v10.14.1
	github.com/gogo/protobuf v1.3.2
	github.com/golang/mock v1.7.0-rc.1
	github.com/golang/protobuf v1.5.3
	github.com/golang/snappy v0.0.4
	github.com/google/go-cmp v0.5.9
	github.com/google/uuid v1.3.0
	github.com/goombaio/namegenerator v0.0.0-20181006234301-989e774b106e
	github.com/grafana-operator/grafana-operator/v4 v4.0.0-00010101000000-000000000000
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.16.2
	github.com/hashicorp/go-hclog v1.4.0
	github.com/hashicorp/go-plugin v1.4.3
	github.com/hashicorp/golang-lru/v2 v2.0.3
	github.com/hashicorp/memberlist v0.5.0
	github.com/iancoleman/strcase v0.3.0
	github.com/imdario/mergo v0.3.13
	github.com/jarcoal/httpmock v1.3.0
	github.com/jedib0t/go-pretty/v6 v6.4.6
	github.com/jhump/protoreflect v1.15.1
	github.com/jwalton/go-supportscolor v1.1.0
	github.com/karlseguin/ccache/v3 v3.0.3
	github.com/klauspost/compress v1.16.7
	github.com/kralicky/gpkg v0.0.0-20220311205216-0d8ea9557555
	github.com/kralicky/kmatch v0.0.0-20230301203314-20f658a0e56c
	github.com/kralicky/ragu v1.0.11-0.20230530191519-84f5aaa5fd69
	github.com/kralicky/totem v1.2.0
	github.com/kralicky/yaml/v3 v3.0.0-20220520012407-b0e7050bd81d
	github.com/lestrrat-go/backoff/v2 v2.0.8
	github.com/lestrrat-go/jwx v1.2.25
	github.com/lithammer/shortuuid v3.0.0+incompatible
	github.com/longhorn/upgrade-responder v0.1.5
	github.com/magefile/mage v1.15.0
	github.com/mattn/go-tty v0.0.4
	github.com/mitchellh/mapstructure v1.5.0
	github.com/muesli/termenv v0.15.2
	github.com/nats-io/nats-server/v2 v2.8.4
	github.com/nats-io/nats.go v1.28.0
	github.com/nats-io/nkeys v0.4.4
	github.com/olebedev/when v0.0.0-20221205223600-4d190b02b8d8
	github.com/onsi/biloba v0.1.5
	github.com/onsi/ginkgo/v2 v2.11.0
	github.com/onsi/gomega v1.27.10
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza v0.0.0-00010101000000-000000000000
	github.com/opencontainers/go-digest v1.0.0
	github.com/opensearch-project/opensearch-go v1.1.0
	github.com/opensearch-project/opensearch-go/v2 v2.0.0-00010101000000-000000000000
	github.com/opentracing-contrib/go-stdlib v1.0.0
	github.com/ory/fosite v0.44.1-0.20230614124936-69300915a423
	github.com/pkg/browser v0.0.0-20210911075715-681adbf594b8
	github.com/pkg/errors v0.9.1
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.66.0
	github.com/prometheus-operator/prometheus-operator/pkg/client v0.66.0
	github.com/prometheus/alertmanager v0.25.1-0.20230505130626-263ca5c9438e
	github.com/prometheus/client_golang v1.16.0
	github.com/prometheus/common v0.44.0
	github.com/prometheus/common/sigv4 v0.1.0
	github.com/prometheus/exporter-toolkit v0.10.0
	github.com/prometheus/procfs v0.10.1
	github.com/prometheus/prometheus v0.44.1-0.20230530154238-dfae954dc113
	github.com/pulumi/pulumi/sdk/v3 v3.68.0
	github.com/qmuntal/stateless v1.6.3
	github.com/rancher/charts-build-scripts v0.0.0-00010101000000-000000000000
	github.com/rancher/kubernetes-provider-detector v0.1.5
	github.com/rancher/wrangler v1.1.1-0.20230419173538-80fdf092be3b
	github.com/samber/lo v1.38.1
	github.com/spf13/afero v1.9.5
	github.com/spf13/cobra v1.7.0
	github.com/spf13/pflag v1.0.5
	github.com/steveteuber/kubectl-graph v0.6.0
	github.com/thanos-io/thanos v0.31.1-0.20230616082957-d43026952989
	github.com/thediveo/enumflag/v2 v2.0.2
	github.com/tidwall/gjson v1.14.4
	github.com/tidwall/sjson v1.2.5
	github.com/ttacon/chalk v0.0.0-20160626202418-22c06c80ed31
	github.com/weaveworks/common v0.0.0-20221201103051-7c2720a9024d
	github.com/zeebo/xxh3 v1.0.2
	go.etcd.io/etcd/api/v3 v3.5.9
	go.etcd.io/etcd/client/v3 v3.5.9
	go.etcd.io/etcd/etcdctl/v3 v3.5.9
	go.opentelemetry.io/collector/pdata v1.0.0-rcv0013
	go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin v0.40.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.42.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.42.0
	go.opentelemetry.io/contrib/propagators/autoprop v0.42.0
	go.opentelemetry.io/otel v1.16.0
	go.opentelemetry.io/otel/exporters/jaeger v1.16.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.16.0
	go.opentelemetry.io/otel/exporters/prometheus v0.39.0
	go.opentelemetry.io/otel/metric v1.16.0
	go.opentelemetry.io/otel/sdk v1.16.0
	go.opentelemetry.io/otel/sdk/metric v0.39.0
	go.opentelemetry.io/otel/trace v1.16.0
	go.opentelemetry.io/proto/otlp v1.0.0
	go.uber.org/atomic v1.11.0
	go.uber.org/multierr v1.11.0
	go.uber.org/zap v1.25.0
	golang.org/x/crypto v0.12.0
	golang.org/x/exp v0.0.0-20230522175609-2e198f4a06a1
	golang.org/x/mod v0.12.0
	golang.org/x/net v0.14.0
	golang.org/x/sync v0.3.0
	golang.org/x/term v0.11.0
	golang.org/x/time v0.3.0
	golang.org/x/tools v0.12.1-0.20230815132531-74c255bcf846
	gonum.org/v1/gonum v0.13.0
	google.golang.org/genproto/googleapis/api v0.0.0-20230815205213-6bfd019c3878
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230815205213-6bfd019c3878
	google.golang.org/grpc v1.57.0
	google.golang.org/protobuf v1.31.0
	gopkg.in/ini.v1 v1.67.0
	gopkg.in/square/go-jose.v2 v2.6.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	k8s.io/api v0.27.4
	k8s.io/apiextensions-apiserver v0.27.2
	k8s.io/apimachinery v0.27.4
	k8s.io/cli-runtime v0.27.1
	k8s.io/client-go v0.27.2
	k8s.io/component-base v0.27.2
	k8s.io/kubectl v0.27.1
	opensearch.opster.io v0.0.0-00010101000000-000000000000
	sigs.k8s.io/controller-runtime v0.15.0
	sigs.k8s.io/node-feature-discovery-operator v0.2.1-0.20230131182250-99b8584e2745
	sigs.k8s.io/yaml v1.3.0
)

require (
	cloud.google.com/go v0.110.7 // indirect
	cloud.google.com/go/compute v1.23.0 // indirect
	cloud.google.com/go/compute/metadata v0.2.3 // indirect
	cloud.google.com/go/iam v1.1.1 // indirect
	cloud.google.com/go/storage v1.30.1 // indirect
	github.com/99designs/go-keychain v0.0.0-20191008050251-8e49817e8af4 // indirect
	github.com/AdaLogics/go-fuzz-headers v0.0.0-20230106234847-43070de90fa1 // indirect
	github.com/AlekSi/pointer v1.0.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.4.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.1.2 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.0.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20230124172434-306776ec8161 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v0.7.0 // indirect
	github.com/BurntSushi/toml v1.3.0 // indirect
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.2.0 // indirect
	github.com/Masterminds/squirrel v1.5.4 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/ProtonMail/go-crypto v0.0.0-20221026131551-cf6655e29de4 // indirect
	github.com/acomagu/bufpipe v1.0.3 // indirect
	github.com/aead/chacha20 v0.0.0-20180709150244-8b13a72661da // indirect
	github.com/agext/levenshtein v1.2.1 // indirect
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20211218093645-b94a6e3cc137 // indirect
	github.com/antonmedv/expr v1.12.5 // indirect
	github.com/apparentlymart/go-textseg/v13 v13.0.0 // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/aws/aws-sdk-go-v2 v1.17.1 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.17.10 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.12.23 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.12.19 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.25 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.19 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.3.26 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.9.19 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.11.25 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.13.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.17.1 // indirect
	github.com/aws/smithy-go v1.13.4 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/benbjohnson/clock v1.3.3 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bgentry/speakeasy v0.1.0 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/bradfitz/gomemcache v0.0.0-20190913173617-a41fca850d0b // indirect
	github.com/briandowns/spinner v1.12.0 // indirect
	github.com/bufbuild/protocompile v0.6.0 // indirect
	github.com/bytedance/sonic v1.9.1 // indirect
	github.com/cenkalti/backoff/v4 v4.2.1 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/chai2010/gettext-go v1.0.2 // indirect
	github.com/charmbracelet/harmonica v0.2.0 // indirect
	github.com/cheggaaa/pb v1.0.29 // indirect
	github.com/chenzhuoyu/base64x v0.0.0-20221115062448-fe3a3abad311 // indirect
	github.com/chromedp/cdproto v0.0.0-20230802225258-3cf4e6d46a89 // indirect
	github.com/chromedp/chromedp v0.9.2 // indirect
	github.com/chromedp/sysutil v1.0.0 // indirect
	github.com/cloudflare/circl v1.3.3 // indirect
	github.com/containerd/console v1.0.3 // indirect
	github.com/containerd/containerd v1.7.2 // indirect
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/cppforlife/go-patch v0.2.0 // indirect
	github.com/creasty/defaults v1.7.0 // indirect
	github.com/cristalhq/jwt/v4 v4.0.2 // indirect
	github.com/cyphar/filepath-securejoin v0.2.3 // indirect
	github.com/danieljoos/wincred v1.1.2 // indirect
	github.com/dave/jennifer v1.6.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.0-20210816181553-5444fa50b93d // indirect
	github.com/dennwc/varint v1.0.0 // indirect
	github.com/dgraph-io/ristretto v0.1.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/djherbis/times v1.5.0 // indirect
	github.com/docker/cli v24.0.2+incompatible // indirect
	github.com/docker/distribution v2.8.2+incompatible // indirect
	github.com/docker/docker v24.0.3+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.7.0 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dsnet/compress v0.0.2-0.20210315054119-f66993602bf5 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/dvsekhvalnov/jose2go v1.5.0 // indirect
	github.com/ecordell/optgen v0.0.9 // indirect
	github.com/edsrzf/mmap-go v1.1.0 // indirect
	github.com/efficientgo/core v1.0.0-rc.2 // indirect
	github.com/efficientgo/tools/extkingpin v0.0.0-20220817170617-6c25e3b627dd // indirect
	github.com/emicklei/go-restful/v3 v3.10.2 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/evanphx/json-patch v5.6.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.6.0 // indirect
	github.com/exponent-io/jsonpath v0.0.0-20151013193312-d6023ce2651d // indirect
	github.com/facette/natsort v0.0.0-20181210072756-2cd4dd1e2dcb // indirect
	github.com/fatih/color v1.15.0 // indirect
	github.com/fatih/structtag v1.2.0 // indirect
	github.com/felixge/fgprof v0.9.3 // indirect
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/flosch/pongo2/v6 v6.0.0 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.2 // indirect
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32 // indirect
	github.com/gin-contrib/sse v0.1.0 // indirect
	github.com/go-errors/errors v1.4.2 // indirect
	github.com/go-git/gcfg v1.5.0 // indirect
	github.com/go-git/go-billy/v5 v5.4.0 // indirect
	github.com/go-git/go-git/v5 v5.6.0 // indirect
	github.com/go-gorp/gorp/v3 v3.0.5 // indirect
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/analysis v0.21.4 // indirect
	github.com/go-openapi/errors v0.20.3 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/loads v0.21.2 // indirect
	github.com/go-openapi/runtime v0.25.0 // indirect
	github.com/go-openapi/spec v0.20.8 // indirect
	github.com/go-openapi/swag v0.22.4 // indirect
	github.com/go-openapi/validate v0.22.1 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-redis/redis/v8 v8.11.5 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.2.1 // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/godbus/dbus v0.0.0-20190726142602-4481cbc300e2 // indirect
	github.com/gofrs/uuid v4.4.0+incompatible // indirect
	github.com/gogo/googleapis v1.4.0 // indirect
	github.com/gogo/status v1.1.1 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.0 // indirect
	github.com/golang-migrate/migrate/v4 v4.16.2 // indirect
	github.com/golang/glog v1.1.1 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/btree v1.0.1 // indirect
	github.com/google/gnostic v0.6.9 // indirect
	github.com/google/go-jsonnet v0.20.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20230817174616-7a8ec2ada47b // indirect
	github.com/google/s2a-go v0.1.4 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.2.3 // indirect
	github.com/googleapis/gax-go/v2 v2.11.0 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/gosuri/uitable v0.0.4 // indirect
	github.com/grafana/regexp v0.0.0-20221122212121-6b5c0a4cb7fd // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.0.0-rc.2.0.20201207153454-9f6bf00c00a7 // indirect
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645 // indirect
	github.com/gsterjov/go-libsecret v0.0.0-20161001094733-a6f4afe4910c // indirect
	github.com/hashicorp/consul/api v1.20.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-msgpack v0.5.5 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.4 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.2 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/hashicorp/go-version v1.6.0 // indirect
	github.com/hashicorp/golang-lru v0.6.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hashicorp/hcl/v2 v2.16.1 // indirect
	github.com/hashicorp/serf v0.10.1 // indirect
	github.com/hashicorp/yamux v0.0.0-20211028200310-0bc27b27de87 // indirect
	github.com/huandu/xstrings v1.4.0 // indirect
	github.com/iancoleman/orderedmap v0.2.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jessevdk/go-flags v1.5.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/jmoiron/sqlx v1.3.5 // indirect
	github.com/jonboulle/clockwork v0.2.2 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/julienschmidt/httprouter v1.3.0 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.4 // indirect
	github.com/knadh/koanf/maps v0.1.1 // indirect
	github.com/knadh/koanf/providers/confmap v0.1.0 // indirect
	github.com/knadh/koanf/v2 v2.0.1 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lann/builder v0.0.0-20180802200727-47ae307949d0 // indirect
	github.com/lann/ps v0.0.0-20150810152359-62de8c46ede0 // indirect
	github.com/leodido/go-urn v1.2.4 // indirect
	github.com/lestrrat-go/blackmagic v1.0.0 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/iter v1.0.1 // indirect
	github.com/lestrrat-go/option v1.0.0 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matryer/is v1.4.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/mattn/goveralls v0.0.12 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mgutz/ansi v0.0.0-20170206155736-9520e82c474b // indirect
	github.com/miekg/dns v1.1.53 // indirect
	github.com/minio/highwayhash v1.0.2 // indirect
	github.com/minio/md5-simd v1.1.2 // indirect
	github.com/minio/minio-go/v7 v7.0.57-0.20230614143930-1d018af3bfab // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/go-ps v1.0.0 // indirect
	github.com/mitchellh/go-testing-interface v1.14.1 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/mtibben/percent v0.2.1 // indirect
	github.com/muesli/ansi v0.0.0-20211018074035-2e021307bc4b // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/muesli/reflow v0.3.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f // indirect
	github.com/nats-io/jwt/v2 v2.3.0 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/ncw/swift v1.0.53 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/observiq/ctimefmt v1.0.0 // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/olekukonko/tablewriter v0.0.5 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/coreinternal v0.80.0 // indirect
	github.com/opencontainers/image-spec v1.1.0-rc4 // indirect
	github.com/openshift/api v3.9.0+incompatible // indirect
	github.com/opentracing-contrib/go-grpc v0.0.0-20210225150812-73cb765af46e // indirect
	github.com/opentracing/basictracer-go v1.1.0 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/ory/go-acc v0.2.9-0.20230103102148-6b1c9a70dbbe // indirect
	github.com/ory/go-convenience v0.1.0 // indirect
	github.com/ory/x v0.0.561 // indirect
	github.com/pborman/uuid v1.2.1 // indirect
	github.com/pelletier/go-toml/v2 v2.0.8 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pjbgf/sha1cd v0.3.0 // indirect
	github.com/pkg/term v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/rancher/lasso v0.0.0-20221227210133-6ea88ca2fbcc // indirect
	github.com/redis/rueidis v1.0.2-go1.18 // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	github.com/rogpeppe/go-internal v1.11.0 // indirect
	github.com/rs/cors v1.9.0 // indirect
	github.com/rs/xid v1.5.0 // indirect
	github.com/rubenv/sql-migrate v1.3.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sabhiram/go-gitignore v0.0.0-20210923224102-525f6e181f06 // indirect
	github.com/santhosh-tekuri/jsonschema/v5 v5.0.0 // indirect
	github.com/sean-/seed v0.0.0-20170313163322-e2103e2c3529 // indirect
	github.com/segmentio/fasthash v1.0.3 // indirect
	github.com/sercand/kuberesolver v2.4.0+incompatible // indirect
	github.com/sergi/go-diff v1.3.1 // indirect
	github.com/shopspring/decimal v1.3.1 // indirect
	github.com/shurcooL/httpfs v0.0.0-20190707220628-8d4bc4ba7749 // indirect
	github.com/shurcooL/vfsgen v0.0.0-20200824052919-0d455de96546 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/skeema/knownhosts v1.1.0 // indirect
	github.com/sony/gobreaker v0.5.0 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/viper v1.16.0 // indirect
	github.com/stretchr/objx v0.5.0 // indirect
	github.com/stretchr/testify v1.8.4 // indirect
	github.com/subosito/gotenv v1.4.2 // indirect
	github.com/texttheater/golang-levenshtein v1.0.1 // indirect
	github.com/thanos-io/objstore v0.0.0-20230201072718-11ffbc490204 // indirect
	github.com/thanos-io/promql-engine v0.0.0-20230526105742-791d78b260ea // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tweekmonster/luser v0.0.0-20161003172636-3fa38070dbd7 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/uber/jaeger-client-go v2.30.0+incompatible // indirect
	github.com/uber/jaeger-lib v2.4.1+incompatible // indirect
	github.com/ugorji/go/codec v1.2.11 // indirect
	github.com/vimeo/galaxycache v0.0.0-20210323154928-b7e5d71c067a // indirect
	github.com/wayneashleyberry/terminal-dimensions v1.1.0 // indirect
	github.com/weaveworks/promrus v1.2.0 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/xhit/go-str2duration/v2 v2.1.0 // indirect
	github.com/xiang90/probing v0.0.0-20190116061207-43a291ad63a2 // indirect
	github.com/xlab/treeprint v1.1.0 // indirect
	github.com/zclconf/go-cty v1.12.1 // indirect
	go.etcd.io/bbolt v1.3.7 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.9 // indirect
	go.etcd.io/etcd/client/v2 v2.305.9 // indirect
	go.etcd.io/etcd/etcdutl/v3 v3.5.9 // indirect
	go.etcd.io/etcd/pkg/v3 v3.5.9 // indirect
	go.etcd.io/etcd/raft/v3 v3.5.9 // indirect
	go.etcd.io/etcd/server/v3 v3.5.9 // indirect
	go.mongodb.org/mongo-driver v1.11.3 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/collector v0.80.0 // indirect
	go.opentelemetry.io/collector/component v0.80.0 // indirect
	go.opentelemetry.io/collector/config/configtelemetry v0.80.0 // indirect
	go.opentelemetry.io/collector/confmap v0.80.1-0.20230629234129-50c94c941969 // indirect
	go.opentelemetry.io/collector/consumer v0.80.0 // indirect
	go.opentelemetry.io/collector/exporter v0.80.0 // indirect
	go.opentelemetry.io/collector/extension v0.80.0 // indirect
	go.opentelemetry.io/collector/featuregate v1.0.0-rcv0013 // indirect
	go.opentelemetry.io/collector/processor v0.80.0 // indirect
	go.opentelemetry.io/collector/receiver v0.80.0 // indirect
	go.opentelemetry.io/contrib/propagators/aws v1.17.0 // indirect
	go.opentelemetry.io/contrib/propagators/b3 v1.17.0 // indirect
	go.opentelemetry.io/contrib/propagators/jaeger v1.17.0 // indirect
	go.opentelemetry.io/contrib/propagators/ot v1.17.0 // indirect
	go.opentelemetry.io/otel/bridge/opentracing v1.16.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/internal/retry v1.16.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.16.0 // indirect
	go.starlark.net v0.0.0-20200901195727-6e684ef5eeee // indirect
	go.uber.org/goleak v1.2.1 // indirect
	go4.org/intern v0.0.0-20230205224052-192e9f60865c // indirect
	go4.org/unsafe/assume-no-moving-gc v0.0.0-20230525183740-e7c30c78aeb2 // indirect
	golang.org/x/arch v0.3.0 // indirect
	golang.org/x/oauth2 v0.10.0 // indirect
	golang.org/x/sys v0.11.0 // indirect
	golang.org/x/text v0.12.0 // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	gomodules.xyz/jsonpatch/v2 v2.3.0 // indirect
	google.golang.org/api v0.126.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20230815205213-6bfd019c3878 // indirect
	gopkg.in/alecthomas/kingpin.v2 v2.2.6 // indirect
	gopkg.in/cheggaaa/pb.v1 v1.0.28 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/telebot.v3 v3.1.3 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	helm.sh/helm/v3 v3.12.0 // indirect
	k8s.io/apiserver v0.27.2 // indirect
	k8s.io/klog/v2 v2.100.1 // indirect
	k8s.io/kube-openapi v0.0.0-20230501164219-8b0f38b5fd1f // indirect
	k8s.io/utils v0.0.0-20230505201702-9f6742963106 // indirect
	lukechampine.com/frand v1.4.2 // indirect
	oras.land/oras-go v1.2.4-0.20230710032925-ae22fd053bb5 // indirect
	sigs.k8s.io/gateway-api v0.6.0 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/kustomize/api v0.13.2 // indirect
	sigs.k8s.io/kustomize/kyaml v0.14.1 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sourcegraph.com/sourcegraph/appdash v0.0.0-20211028080628-e2786a622600 // indirect
)

replace github.com/openshift/api => github.com/openshift/api v0.0.0-20210216211028-bb81baaf35cd

// Forks
replace (
	github.com/NVIDIA/gpu-operator => github.com/kralicky/gpu-operator v1.8.1-0.20211112183255-72529edf38be
	github.com/cortexproject/cortex => github.com/kralicky/cortex v1.16.0-opni.0
	github.com/grafana-operator/grafana-operator/v4 => github.com/kralicky/grafana-operator/v4 v4.2.1-0.20230523174150-7468e11cb5a1
	// https://github.com/hashicorp/go-plugin/pull/251
	github.com/hashicorp/go-plugin => github.com/alexandreLamarre/go-plugin v0.1.1-0.20230417174342-eab684801be5
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza => github.com/dbason/opentelemetry-collector-contrib/pkg/stanza v0.0.0-20230704041503-6f0f267c2d92
	github.com/opensearch-project/opensearch-go/v2 => github.com/dbason/opensearch-go/v2 v2.0.0-20221202021211-6aec8f80bc41
	github.com/rancher/charts-build-scripts => github.com/dbason/charts-build-scripts v0.3.4-0.20220429024555-807c076e8116
	go.uber.org/zap => github.com/kralicky/zap v1.24.1-0.20230718165024-a2256218e4cc
	google.golang.org/protobuf => github.com/kralicky/protobuf-go v0.0.0-20230320172414-3920b92ef96f
	opensearch.opster.io => github.com/dbason/opensearch-k8s-operator/opensearch-operator v0.0.0-20230809020457-b60fd235fc1d
)

// Cortex replacements (copied from cortex go.mod)
replace (
	// Override since git.apache.org is down.  The docs say to fetch from github.
	git.apache.org/thrift.git => github.com/apache/thrift v0.0.0-20180902110319-2566ecd5d999

	// Using a 3rd-party branch for custom dialer - see https://github.com/bradfitz/gomemcache/pull/86
	github.com/bradfitz/gomemcache => github.com/themihai/gomemcache v0.0.0-20180902122335-24332e2d58ab

	// v0.20.5 caused go mod checksum error
	github.com/go-openapi/spec => github.com/go-openapi/spec v0.20.6

	// Use fork of gocql that has gokit logs and Prometheus metrics.
	github.com/gocql/gocql => github.com/grafana/gocql v0.0.0-20200605141915-ba5dc39ece85

	github.com/google/gnostic => github.com/googleapis/gnostic v0.6.9

	// Replace memberlist with Grafana's fork which includes some fixes that haven't been merged upstream yet
	github.com/hashicorp/memberlist => github.com/grafana/memberlist v0.2.5-0.20211201083710-c7bc8e9df94b

	// the v6.0.5851 Prometheus depends on doesn't seem to exist anymore?
	github.com/ionos-cloud/sdk-go/v6 => github.com/ionos-cloud/sdk-go/v6 v6.0.4

	// Same version being used by thanos
	github.com/vimeo/galaxycache => github.com/thanos-community/galaxycache v0.0.0-20211122094458-3a32041a1f1e

	// Same replace used by thanos: (may be removed in the future)
	// https://github.com/thanos-io/thanos/blob/fdeea3917591fc363a329cbe23af37c6fff0b5f0/go.mod#L265
	gopkg.in/alecthomas/kingpin.v2 => github.com/alecthomas/kingpin v1.3.8-0.20210301060133-17f40c25f497
)

replace (
	github.com/banzaicloud/logging-operator => github.com/kralicky/logging-operator v0.0.0-20230206225216-304d63447c1f
	github.com/banzaicloud/logging-operator/pkg/sdk => github.com/kralicky/logging-operator/pkg/sdk v0.0.0-20230206225216-304d63447c1f
	github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/syslogng/config => github.com/kralicky/logging-operator/pkg/sdk/logging/model/syslogng/config v0.0.0-20230206225216-304d63447c1f
)

// Temporary replacements
replace (
	github.com/samber/lo => github.com/samber/lo v1.36.1-0.20230320154156-56ef8fe8a306
	github.com/sercand/kuberesolver => github.com/sercand/kuberesolver/v4 v4.0.0
)

// Version pins
replace github.com/thanos-io/thanos => github.com/thanos-io/thanos v0.31.1-0.20230629090141-7749936b7313

replace github.com/prometheus/prometheus => github.com/prometheus/prometheus v0.44.1-0.20230530154238-dfae954dc113

replace github.com/prometheus/alertmanager => github.com/prometheus/alertmanager v0.25.1-0.20230505130626-263ca5c9438e

// Exclude old koanf version
exclude github.com/knadh/koanf v1.5.0
