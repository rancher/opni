package v1_test

import (
	"fmt"

	"github.com/bufbuild/protovalidate-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	lo "github.com/samber/lo"
	"github.com/ttacon/chalk"

	v1 "github.com/rancher/opni/pkg/config/v1"
	"github.com/rancher/opni/pkg/test/testdata"
	"github.com/rancher/opni/pkg/util/flagutil"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/protobuf/proto"
)

var _ = Describe("Gateway Config", Label("unit"), Ordered, func() {
	leafCertData := string(testdata.TestData("localhost.crt"))
	leafKeyData := string(testdata.TestData("localhost.key"))
	caCertData := string(testdata.TestData("root_ca.crt"))

	var v *protovalidate.Validator
	BeforeAll(func() {
		v = validation.MustNewValidator(
			protovalidate.WithCoverage(true),
		)
	})
	AfterAll(func() {
		report, err := v.GenerateCoverageReport()

		for constraint, evals := range report.ByUniqueConstraint {
			name := constraint.Descriptor().FullName()
			fmt.Fprintf(GinkgoWriter, "[%s]:\n", name)
			for _, eval := range evals {
				hit := eval.HitCount > 0
				var msg string
				if hit {
					msg = chalk.Green.Color(fmt.Sprintf("Hits: %d", eval.HitCount))
				} else {
					msg = chalk.Red.Color("No Coverage")
				}
				fmt.Fprintf(GinkgoWriter, "  [%T] %s: %s\n", eval.Evaluator, eval.ContainingDesc.FullName(), msg)
			}
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(report).NotTo(BeNil())
	})
	DescribeTable("Validation",
		func(msg proto.Message, expectedErrs ...string) {
			err := v.Validate(msg)
			if len(expectedErrs) == 0 {
				Expect(err).NotTo(HaveOccurred())
			} else {
				Expect(err).To(HaveOccurred())
				for _, expectedErr := range expectedErrs {
					Expect(err.Error()).To(ContainSubstring(expectedErr))
				}
				numViolations := len(err.(*protovalidate.ValidationError).Violations)
				Expect(numViolations).To(Equal(len(expectedErrs)),
					"expected %d errors, got %d:\n%s", len(expectedErrs), numViolations, err.Error())
			}
		},

		// Listen Addresses
		Entry("Listen Addresses (Server/Server)", withDefaults(func(cfg *v1.GatewayConfigSpec) {
			cfg.Storage.Etcd = &v1.EtcdSpec{Endpoints: []string{"localhost:2379"}}
			*cfg.Server.HttpListenAddress = *cfg.Server.GrpcListenAddress
		}), "[check_conflicting_addresses]"),
		Entry("Listen Addresses (Management/Management)", withDefaults(func(cfg *v1.GatewayConfigSpec) {
			cfg.Storage.Etcd = &v1.EtcdSpec{Endpoints: []string{"localhost:2379"}}
			*cfg.Management.HttpListenAddress = *cfg.Management.GrpcListenAddress
		}), "[check_conflicting_addresses]"),
		Entry("Listen Addresses (Relay/Server)", withDefaults(func(cfg *v1.GatewayConfigSpec) {
			cfg.Storage.Etcd = &v1.EtcdSpec{Endpoints: []string{"localhost:2379"}}
			*cfg.Relay.GrpcListenAddress = *cfg.Server.GrpcListenAddress
		}), "[check_conflicting_addresses]"),
		Entry("Listen Addresses (Relay/Management)", withDefaults(func(cfg *v1.GatewayConfigSpec) {
			cfg.Storage.Etcd = &v1.EtcdSpec{Endpoints: []string{"localhost:2379"}}
			*cfg.Relay.GrpcListenAddress = *cfg.Management.GrpcListenAddress
		}), "[check_conflicting_addresses]"),
		Entry("Listen Addresses (Server/Management)", withDefaults(func(cfg *v1.GatewayConfigSpec) {
			cfg.Storage.Etcd = &v1.EtcdSpec{Endpoints: []string{"localhost:2379"}}
			*cfg.Server.GrpcListenAddress = *cfg.Management.GrpcListenAddress
		}), "[check_conflicting_addresses]"),
		Entry("Listen Addresses (Server/Relay)", withDefaults(func(cfg *v1.GatewayConfigSpec) {
			cfg.Storage.Etcd = &v1.EtcdSpec{Endpoints: []string{"localhost:2379"}}
			*cfg.Server.GrpcListenAddress = *cfg.Relay.GrpcListenAddress
		}), "[check_conflicting_addresses]"),
		Entry("Listen Addresses (Management/Relay)", withDefaults(func(cfg *v1.GatewayConfigSpec) {
			cfg.Storage.Etcd = &v1.EtcdSpec{Endpoints: []string{"localhost:2379"}}
			*cfg.Management.GrpcListenAddress = *cfg.Relay.GrpcListenAddress
		}), "[check_conflicting_addresses]"),
		Entry("Server Addresses", &v1.ServerSpec{
			HttpListenAddress: lo.ToPtr("0.0.0.0.0:65537"),
			GrpcListenAddress: lo.ToPtr("unix://127.0.0.1:9090"),
		}, "httpListenAddress: invalid IP address", "grpcListenAddress: address unix://127.0.0.1:9090: too many colons in address"),
		Entry("Server Addresses", &v1.ServerSpec{
			HttpListenAddress: lo.ToPtr("http://localhost:8080"),
			GrpcListenAddress: lo.ToPtr("0.0.0.0:65537"),
		}, "httpListenAddress: address http://localhost:8080: too many colons in address", "grpcListenAddress: port number out of range"),
		Entry("Server Addresses", &v1.ServerSpec{
			HttpListenAddress: lo.ToPtr("localhost:8080"),
			GrpcListenAddress: lo.ToPtr("unix:///tmp/sock"),
		}),
		Entry("Management Addresses", &v1.ManagementServerSpec{
			HttpListenAddress: lo.ToPtr("0.0.0.0.0:65537"),
			GrpcListenAddress: lo.ToPtr("unix://127.0.0.1:9090"),
			AdvertiseAddress:  lo.ToPtr("0.0.0.0:0"),
		},
			"httpListenAddress: invalid IP address",
			"grpcListenAddress: address unix://127.0.0.1:9090: too many colons in address",
			"[mgmt_grpc_advertise_address_port]",
		),
		Entry("Management Addresses", &v1.ManagementServerSpec{
			HttpListenAddress: lo.ToPtr("127.0.0.1:12345"),
			GrpcListenAddress: lo.ToPtr("127.0.0.1:12346"),
			AdvertiseAddress:  lo.ToPtr("127.0.0.1:12347"),
		}),
		Entry("Relay Addresses", &v1.RelayServerSpec{
			GrpcListenAddress: lo.ToPtr("unix://127.0.0.1:9090"),
			AdvertiseAddress:  lo.ToPtr("0.0.0.0:0"),
		},
			"grpcListenAddress: address unix://127.0.0.1:9090: too many colons in address",
			"[relay_grpc_advertise_address_port]",
		),
		Entry("Relay Addresses", &v1.RelayServerSpec{
			GrpcListenAddress: lo.ToPtr("127.0.0.1:12345"),
			AdvertiseAddress:  lo.ToPtr("127.0.0.1:12346"),
		}),
		Entry("Health Server Addresses", &v1.HealthServerSpec{
			HttpListenAddress: lo.ToPtr("z"),
		}, "httpListenAddress: address z: missing port in address"),
		Entry("Health Server Addresses", &v1.HealthServerSpec{
			HttpListenAddress: lo.ToPtr("z:"),
		}, "httpListenAddress: invalid IP address"),
		Entry("Health Server Addresses", &v1.HealthServerSpec{
			HttpListenAddress: lo.ToPtr("localhost:"),
		}),
		Entry("Health Server Addresses", &v1.HealthServerSpec{
			HttpListenAddress: lo.ToPtr("localhost:0"),
		}),
		Entry("Dashboard Server Addresses", &v1.DashboardServerSpec{
			HttpListenAddress: lo.ToPtr("0.0.0.0.0:65537"),
			AdvertiseAddress:  lo.ToPtr(":0"),
		}, "httpListenAddress: invalid IP address", "[dashboard_http_advertise_address_port]"),
		Entry("Dashboard Server Hostname", &v1.DashboardServerSpec{
			Hostname: lo.ToPtr("localhost"),
		}),
		Entry("Dashboard Server Hostname", &v1.DashboardServerSpec{
			Hostname: lo.ToPtr("localhost:8080"),
		}, "hostname: value must be a valid hostname"),
		Entry("Dashboard Server Hostname", &v1.DashboardServerSpec{
			Hostname: lo.ToPtr("localhost:"),
		}, "hostname: value must be a valid hostname"),
		Entry("Dashboard Server Hostname", &v1.DashboardServerSpec{
			Hostname: lo.ToPtr(""),
		}, "hostname: value must be a valid hostname"),
		Entry("Dashboard Server Trusted Proxies", &v1.DashboardServerSpec{
			TrustedProxies: []string{"localhost"},
		}, "must be a valid IP address or CIDR"),
		Entry("Dashboard Server Trusted Proxies", &v1.DashboardServerSpec{
			TrustedProxies: []string{"192.168.1.1/33"},
		}, "must be a valid IP address or CIDR"),
		Entry("Dashboard Server Trusted Proxies", &v1.DashboardServerSpec{
			TrustedProxies: []string{"10.0.0.0/8", "192.168.1.0/24"},
		}),

		// Storage
		Entry("Storage: Etcd", &v1.StorageSpec{
			Backend: lo.ToPtr(v1.StorageBackend_Etcd),
			Etcd:    &v1.EtcdSpec{},
		}, "etcd.endpoints: value must contain at least 1 item(s)"),
		Entry("Storage: Etcd", &v1.StorageSpec{
			Backend: lo.ToPtr(v1.StorageBackend_JetStream),
			Etcd:    &v1.EtcdSpec{},
		}, "selected storage backend must have matching configuration set"),
		Entry("Storage: Etcd", &v1.EtcdSpec{
			Endpoints: []string{"localhost:2379"},
			Certs: &v1.MTLSSpec{
				ServerCAData:   &caCertData,
				ClientCertData: &leafCertData,
				ClientKeyData:  &leafKeyData,
			},
		}),
		Entry("Storage: Etcd", &v1.EtcdSpec{
			Endpoints: []string{"localhost:2379", "http://localhost:2380", "aaaa", "aaaa"},
		}, "endpoints: repeated value must contain unique items"),
		Entry("Storage: Etcd", &v1.EtcdSpec{
			Endpoints: []string{"localhost:2379", "http://localhost:2380", "aaaa", "http://\x7f"},
		}, "endpoints[3]: value must be a valid URI"),
		Entry("Storage: Jetstream", &v1.StorageSpec{
			Backend:   lo.ToPtr(v1.StorageBackend_JetStream),
			JetStream: &v1.JetStreamSpec{},
		}, "jetStream.endpoint: value is required", "jetStream.nkeySeedPath: value is required"),
		Entry("Storage: Jetstream", &v1.StorageSpec{
			Backend:   lo.ToPtr(v1.StorageBackend_Etcd),
			JetStream: &v1.JetStreamSpec{},
		}, "selected storage backend must have matching configuration set"),

		// MTLS
		Entry("MTLS", &v1.MTLSSpec{
			ServerCA:     lo.ToPtr("/path/to/crt"),
			ServerCAData: &caCertData,
		}, "[fields_mutually_exclusive_serverca]"),
		Entry("MTLS", &v1.MTLSSpec{
			ClientCA:     lo.ToPtr("/path/to/crt"),
			ClientCAData: &caCertData,
		}, "[fields_mutually_exclusive_clientca]"),
		Entry("MTLS", &v1.MTLSSpec{
			ClientCert:     lo.ToPtr("/path/to/crt"),
			ClientCertData: &leafCertData,
		}, "[fields_mutually_exclusive_clientcert]"),
		Entry("MTLS", &v1.MTLSSpec{
			ClientKey:     lo.ToPtr("/path/to/key"),
			ClientKeyData: &leafKeyData,
		}, "[fields_mutually_exclusive_clientkey]"),

		Entry("MTLS", &v1.MTLSSpec{
			ServerCAData: lo.ToPtr("invalid x509 data"),
		}, "[x509_server_ca_data]"),
		Entry("MTLS", &v1.MTLSSpec{
			ClientCAData: lo.ToPtr("invalid x509 data"),
		}, "[x509_client_ca_data]"),
		Entry("MTLS", &v1.MTLSSpec{
			ClientCertData: lo.ToPtr("invalid x509 data"),
		}, "[x509_client_cert_data]"),
		Entry("MTLS", &v1.MTLSSpec{
			ClientKeyData: lo.ToPtr("invalid pem data"),
		}, "[pem_client_key_data]"),

		Entry("MTLS", &v1.MTLSSpec{
			ServerCAData:   &caCertData,
			ClientCertData: &leafCertData,
			ClientKeyData:  &leafKeyData,
		}),
		Entry("MTLS", &v1.MTLSSpec{
			ServerCAData:   &leafCertData,
			ClientCertData: &caCertData,
			ClientKeyData:  &leafKeyData,
		}, "x509: invalid signature: parent certificate cannot sign this kind of certificate"),
		Entry("MTLS", &v1.MTLSSpec{
			ServerCAData:   &leafKeyData,
			ClientCertData: &caCertData,
			ClientKeyData:  &leafCertData,
		}, "x509: malformed tbs certificate", "serverCAData: x509: malformed tbs certificate"),

		// Jetstream
		Entry("Jetstream", &v1.JetStreamSpec{
			Endpoint: lo.ToPtr("localhost:4222"),
		}, "nkeySeedPath: value is required"),
		Entry("Jetstream", &v1.JetStreamSpec{
			Endpoint:     lo.ToPtr("localhost:4222"),
			NkeySeedPath: lo.ToPtr("/path/to/nkey/seed"),
		}),
		Entry("Jetstream", &v1.JetStreamSpec{
			Endpoint:     lo.ToPtr("localhost:\x7f"),
			NkeySeedPath: lo.ToPtr("/path/to/nkey/seed"),
		}, "endpoint: value must be a valid URI"),

		// Certs
		Entry("Certs", &v1.CertsSpec{
			CaCert:     lo.ToPtr("/path/to/crt"),
			CaCertData: &caCertData,
		}, "[fields_mutually_exclusive_ca]"),
		Entry("Certs", &v1.CertsSpec{
			ServingCert:     lo.ToPtr("/path/to/crt"),
			ServingCertData: &leafCertData,
		}, "[fields_mutually_exclusive_servingcert]"),
		Entry("Certs", &v1.CertsSpec{
			ServingKey:     lo.ToPtr("/path/to/key"),
			ServingKeyData: &leafKeyData,
		}, "[fields_mutually_exclusive_servingkey]"),

		Entry("Certs", &v1.CertsSpec{
			CaCertData: lo.ToPtr("invalid x509 data"),
		}, "[x509_ca_cert_data]"),
		Entry("Certs", &v1.CertsSpec{
			ServingCertData: lo.ToPtr("invalid x509 data"),
		}, "[x509_serving_cert_data]"),
		Entry("Certs", &v1.CertsSpec{
			ServingKeyData: lo.ToPtr("invalid pem data"),
		}, "[pem_serving_key_data]"),

		Entry("Certs", &v1.CertsSpec{
			CaCertData:      &caCertData,
			ServingCertData: &leafCertData,
			ServingKeyData:  &leafKeyData,
		}),
		Entry("Certs", &v1.CertsSpec{
			CaCertData:      &leafCertData,
			ServingCertData: &caCertData,
			ServingKeyData:  &leafKeyData,
		}, "x509: invalid signature: parent certificate cannot sign this kind of certificate"),
		Entry("Certs", &v1.CertsSpec{
			CaCertData:      &leafKeyData,
			ServingCertData: &caCertData,
			ServingKeyData:  &leafCertData,
		}, "x509: malformed tbs certificate", "caCertData: x509: malformed tbs certificate"),

		// Plugins
		Entry("Plugins", &v1.PluginsSpec{
			Dir: lo.ToPtr("/foo/bar/baz"),
			Cache: &v1.CacheSpec{
				Filesystem: &v1.FilesystemCacheSpec{
					Dir: lo.ToPtr("/foo/bar/baz"),
				},
			},
		}, "[plugin_dirs_unique]"),
		Entry("Plugins", &v1.PluginsSpec{
			Dir: lo.ToPtr("/foo/bar/baz"),
			Cache: &v1.CacheSpec{
				Filesystem: &v1.FilesystemCacheSpec{
					Dir: lo.ToPtr("/foo/bar/cache"),
				},
			},
		}),
		Entry("Plugins", &v1.PluginFilters{
			Exclude: []string{
				"github.com/foo/bar",
				"foo/bar",
				"bar",
			},
		},
			`exclude[1]: malformed module path "foo/bar": missing dot in first path element [go_module_path]`,
			`exclude[2]: malformed module path "bar": missing dot in first path element [go_module_path]`,
		),

		// Plugin Cache
		Entry("Plugin Cache", &v1.CacheSpec{
			PatchEngine: lo.ToPtr(v1.PatchEngine_Bsdiff),
			Backend:     lo.ToPtr(v1.CacheBackend_Filesystem),
			Filesystem:  &v1.FilesystemCacheSpec{},
		}, "dir: value is required"),

		// Keyring
		Entry("Keyring", &v1.KeyringSpec{
			RuntimeKeyDirs: []string{"/foo/bar/baz", "/foo/bar/baz"},
		}, "runtimeKeyDirs: repeated value must contain unique items"),
	)
})

func withDefaults[T any, PT interface {
	*T
	flagutil.FlagSetter
}](fn func(t PT)) PT {
	var pt PT = new(T)
	flagutil.LoadDefaults(pt)
	fn(pt)
	return pt
}
