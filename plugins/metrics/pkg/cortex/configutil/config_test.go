package configutil_test

import (
	"flag"

	"github.com/cortexproject/cortex/pkg/cortex"
	"github.com/go-kit/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/util/flagutil"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"github.com/rancher/opni/plugins/metrics/pkg/cortex/configutil"
	"gopkg.in/yaml.v2"
)

func loadAndValidateConfig(yamlData []byte) error {
	GinkgoHelper()

	var cfg cortex.Config
	flagset := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg.RegisterFlags(flagset)
	Expect(yaml.UnmarshalStrict(yamlData, &cfg)).To(Succeed())
	Expect(flagset.Parse([]string{})).To(Succeed())

	return cfg.Validate(log.NewNopLogger())
}

var allTargets = []string{
	"distributor",
	"query-frontend",
	"purger",
	"ruler",
	"compactor",
	"store-gateway",
	"ingester",
	"alertmanager",
	"querier",
}

var _ = Describe("Config", func() {
	It("should generate a valid default config", func() {
		appconfig := &cortexops.CortexApplicationConfig{}
		flagutil.LoadDefaults(appconfig)
		conf, _, err := configutil.CortexAPISpecToCortexConfig[*cortexops.CortexApplicationConfig](appconfig, configutil.NewTargetsOverride("all")...)
		Expect(err).NotTo(HaveOccurred())
		Expect(conf.Target).To(ConsistOf("all"))
		yamlData, err := configutil.MarshalCortexConfig(conf)
		Expect(err).NotTo(HaveOccurred())

		Expect(loadAndValidateConfig(yamlData)).To(Succeed())
	})
	It("should generate a valid config with overrides", func() {
		appconfig := &cortexops.CortexApplicationConfig{}
		flagutil.LoadDefaults(appconfig)

		conf, _, err := configutil.CortexAPISpecToCortexConfig[*cortexops.CortexApplicationConfig](appconfig,
			configutil.MergeOverrideLists(
				configutil.NewTargetsOverride("all"),
				configutil.NewStandardOverrides(configutil.StandardOverridesShape{
					HttpListenAddress: "127.0.0.1",
					HttpListenNetwork: "tcp",
					HttpListenPort:    8080,
					GrpcListenNetwork: "tcp",
					GrpcListenAddress: "127.0.0.1",
					GrpcListenPort:    9095,
					StorageDir:        "/dev/null",
					RuntimeConfig:     "/dev/null",
				}),
			)...,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(conf.Target).To(ConsistOf("all"))
		yamlData, err := configutil.MarshalCortexConfig(conf)
		Expect(err).NotTo(HaveOccurred())

		Expect(loadAndValidateConfig(yamlData)).To(Succeed())
	})
	It("should generate a valid config with extra overrides", func() {
		appconfig := &cortexops.CortexApplicationConfig{}
		flagutil.LoadDefaults(appconfig)

		conf, _, err := configutil.CortexAPISpecToCortexConfig[*cortexops.CortexApplicationConfig](appconfig,
			configutil.MergeOverrideLists(
				configutil.NewTargetsOverride(allTargets...),
				configutil.NewStandardOverrides(configutil.StandardOverridesShape{
					HttpListenAddress: "127.0.0.1",
					HttpListenNetwork: "tcp",
					HttpListenPort:    8080,
					GrpcListenNetwork: "tcp",
					GrpcListenAddress: "127.0.0.1",
					GrpcListenPort:    9095,
					StorageDir:        "/dev/null",
					RuntimeConfig:     "/dev/null",
				}),
				configutil.NewAutomaticHAOverrides(),
				configutil.NewImplementationSpecificOverrides(configutil.ImplementationSpecificOverridesShape{
					QueryFrontendAddress: "http://localhost:9095",
					MemberlistJoinAddrs:  []string{"localhost"},
					AlertmanagerURL:      "http://localhost:9093",
				}),
			)...,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(conf.Target).To(ConsistOf(allTargets))
		yamlData, err := configutil.MarshalCortexConfig(conf)
		Expect(err).NotTo(HaveOccurred())

		Expect(loadAndValidateConfig(yamlData)).To(Succeed())
	})
})
