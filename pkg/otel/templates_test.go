package otel_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"text/template"
	"time"

	// "github.com/lightstep/otel-config-validator/validator"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/model"
	promcfg "github.com/prometheus/prometheus/config"
	"github.com/rancher/opni/pkg/otel"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/testdata"
	"github.com/rancher/opni/pkg/util"
	"google.golang.org/protobuf/types/known/durationpb"
	"gopkg.in/yaml.v2"
)

func filterByTemplateDefinition(t *template.Template, prefix string) (names []string) {
	names = []string{}
	tmpls := reflect.ValueOf(t).Elem().FieldByName("common").Elem().FieldByName("tmpl").MapRange()
	for tmpls.Next() {
		key := tmpls.Key().String()
		Expect(key).NotTo(BeEmpty())
		if strings.HasPrefix(key, prefix) && !strings.HasSuffix(key, "tmpl") {
			names = append(names, key)
		}
	}
	return
}

var _ = Describe("Otel template functions", Label("unit"), func() {
	When("we use otel template functions", func() {
		It("should convert proto durations to strings", func() {
			s := otel.ProtoDurToString(durationpb.New(time.Second * 5))
			Expect(s).To(Equal("5s"))

			t := otel.OTELTemplates
			tmpl := template.Must(t.Parse(`{{protoDurToString .}}`))
			var b bytes.Buffer
			err := tmpl.Execute(&b, durationpb.New(time.Second*5))
			Expect(err).To(BeNil())
			Expect(b.String()).To(Equal("5s"))
		})

		It("should indent strings", func() {
			t := otel.OTELTemplates
			tmpl := template.Must(t.Parse(`{{indent 2 .}}`))
			var b bytes.Buffer
			err := tmpl.Execute(&b, "test")
			Expect(err).To(BeNil())
			Expect(b.String()).To(Equal("  test"))

			tmpl = template.Must(t.Parse(`{{nindent 8 .}}`))
			b.Reset()
			err = tmpl.Execute(&b, "test\ntest2")
			Expect(err).To(BeNil())
			Expect(b.String()).To(Equal("\n        test\n        test2"))
		})
	})
})

var _ = Describe("Otel config template generation", Label("unit"), Ordered, func() {
	var t *template.Template
	BeforeAll(func() {
		t = util.Must(otel.OTELTemplates.ParseFS(testdata.TestDataFS, "testdata/otel/*.tmpl"))
	})
	When("we use the base templates...", func() {
		Specify("with metrics enabled", func() {
			scrapeCfg := []yaml.MapSlice{}
			out, err := yaml.Marshal([]promcfg.ScrapeConfig{
				{
					JobName:        "test",
					ScrapeInterval: model.Duration(time.Minute),
				},
				{
					JobName:        "test2",
					ScrapeInterval: model.Duration(time.Minute),
				},
			})
			Expect(err).To(Succeed())

			err = yaml.Unmarshal(out, &scrapeCfg)
			Expect(err).To(Succeed())

			config := otel.NodeConfig{
				Metrics: otel.MetricsConfig{
					Enabled:             true,
					ListenPort:          8888,
					RemoteWriteEndpoint: "http://localhost:1234",
					DiscoveredScrapeCfg: otel.PromCfgToString(scrapeCfg),
					Spec: &otel.OTELSpec{
						HostMetrics: true,
						AdditionalScrapeConfigs: []*otel.ScrapeConfig{
							{
								JobName:        "test",
								Targets:        []string{"localhost:3333"},
								ScrapeInterval: "1m",
							},
						},
					},
				},
				Containerized: true,
			}
			tmpls := filterByTemplateDefinition(t, "metrics")
			Expect(tmpls).NotTo(HaveLen(0))

			for _, tmplName := range tmpls {
				tmpl, err := t.Parse(`{{template "` + tmplName + `" .}}`)
				Expect(err).To(BeNil())
				var b bytes.Buffer
				err = tmpl.Execute(&b, config)
				Expect(err).To(BeNil())
				Expect(b.String()).NotTo(BeEmpty(), fmt.Sprintf("template %s should not be empty", tmplName))
			}
		})
		Specify("with metrics disabled", func() {
			config := otel.AggregatorConfig{
				Metrics: otel.MetricsConfig{
					Enabled:             false,
					ListenPort:          8888,
					RemoteWriteEndpoint: "http://localhost:1234",
				},
			}
			tmpls := filterByTemplateDefinition(t, "metrics")
			Expect(tmpls).NotTo(HaveLen(0))

			for _, tmplName := range tmpls {
				tmpl, err := t.Parse(`{{template "` + tmplName + `" .}}`)
				Expect(err).To(BeNil())
				var b bytes.Buffer
				err = tmpl.Execute(&b, config)
				Expect(err).To(BeNil())
				Expect(strings.TrimSpace(b.String())).To(BeEmpty(), fmt.Sprintf("template %s should be empty", tmplName))
			}
		})
	})

})

var _ = Describe("Otel config templates integration", Label("integration"), Ordered, func() {
	var step = "test"
	var configBytes = []byte{}
	var t *template.Template

	debug := func(fileName string, content []byte) {
		if _, ok := os.LookupEnv("DEBUG_OTEL"); ok {
			err := os.WriteFile(fileName+".yaml", configBytes, 0644)
			Expect(err).To(BeNil())
		}
	}
	BeforeAll(func() {
		t = util.Must(otel.OTELTemplates.ParseFS(testdata.TestDataFS, "testdata/otel/*.tmpl"))
	})

	When("we use the node template", func() {
		Specify("with metrics enabled", func() {
			step = "node-metrics"
			validCfgs := []test.TestNodeConfig{
				{
					ReceiverFile:      "./receiver.yaml",
					AggregatorAddress: "localhost:1234",
					Instance:          "test",
					Logs:              otel.LoggingConfig{},
					Metrics: otel.MetricsConfig{
						Enabled:             true,
						ListenPort:          8888,
						RemoteWriteEndpoint: "http://localhost:1234",
						Spec: &otel.OTELSpec{
							HostMetrics: true,
							AdditionalScrapeConfigs: []*otel.ScrapeConfig{
								{
									JobName:        "test",
									Targets:        []string{"localhost:3333"},
									ScrapeInterval: "1m",
								},
							},
						},
					},
				},
				{
					ReceiverFile:      "./receiver.yaml",
					Instance:          "test",
					AggregatorAddress: "localhost:1234",
					Logs:              otel.LoggingConfig{},
					Metrics: otel.MetricsConfig{
						Enabled:             true,
						ListenPort:          8888,
						RemoteWriteEndpoint: "http://localhost:1234",
						Spec: &otel.OTELSpec{
							HostMetrics: false,
							AdditionalScrapeConfigs: []*otel.ScrapeConfig{
								{
									JobName:        "test",
									Targets:        []string{"localhost:3333"},
									ScrapeInterval: "1m",
								},
							},
						},
					},
				},
			}
			receiverTmpl, err := t.Parse(`{{template "node-receivers" .}}`)
			Expect(err).To(BeNil())

			for i, cfg := range validCfgs {
				var b bytes.Buffer
				err = receiverTmpl.Execute(&b, cfg)
				Expect(err).To(BeNil())
				Expect(strings.TrimSpace(b.String())).NotTo(BeEmpty(), fmt.Sprintf("template %s should not be empty", util.Must(json.Marshal(cfg))))
				configBytes = b.Bytes()
				debug(fmt.Sprintf("%s-%d", "receivers", i), configBytes)
			}

			mainTmpl, err := t.Parse(`{{template "node-config" .}}`)
			Expect(err).To(BeNil())
			for i, cfg := range validCfgs {
				var b bytes.Buffer
				err = mainTmpl.Execute(&b, cfg)
				Expect(err).To(BeNil())
				Expect(strings.TrimSpace(b.String())).NotTo(BeEmpty())
				configBytes = b.Bytes()
				// cfg, err := validator.IsValid(configBytes)
				debug(fmt.Sprintf("%s-%d", step, i), configBytes)
				// Expect(err).To(Succeed())
				// Expect(cfg).NotTo(BeNil())
			}
		})
	})

	When("we use the aggregator template...", func() {
		Specify("with metrics enabled", func() {
			step = "aggregator-metrics"
			scrapeCfg := []yaml.MapSlice{}
			out, err := yaml.Marshal([]promcfg.ScrapeConfig{
				{
					JobName:        "test",
					ScrapeInterval: model.Duration(time.Minute),
				},
				{
					JobName:        "test2",
					ScrapeInterval: model.Duration(time.Minute),
				},
			})
			Expect(err).To(Succeed())

			err = yaml.Unmarshal(out, &scrapeCfg)
			Expect(err).To(Succeed())

			validCfgs := []test.TestAggregatorConfig{
				{
					AggregatorAddress: "localhost:1234",
					AgentEndpoint:     "http://localhost:7777",
					Metrics: otel.MetricsConfig{
						Enabled:             true,
						ListenPort:          8888,
						RemoteWriteEndpoint: "http://localhost:1234",
						DiscoveredScrapeCfg: otel.PromCfgToString(scrapeCfg),
						Spec: &otel.OTELSpec{
							HostMetrics: true,
							AdditionalScrapeConfigs: []*otel.ScrapeConfig{
								{
									JobName:        "test",
									Targets:        []string{"localhost:3333"},
									ScrapeInterval: "1m",
								},
							},
						},
					},
				},
				{
					AggregatorAddress: "localhost:1234",
					AgentEndpoint:     "http://localhost:7777",
					Metrics: otel.MetricsConfig{
						Enabled:             true,
						ListenPort:          8888,
						RemoteWriteEndpoint: "http://localhost:1234",
						WALDir:              "/tmp/wal",
						DiscoveredScrapeCfg: otel.PromCfgToString(scrapeCfg),
						Spec: &otel.OTELSpec{
							HostMetrics: true,
							AdditionalScrapeConfigs: []*otel.ScrapeConfig{
								{
									JobName:        "test",
									Targets:        []string{"localhost:3333"},
									ScrapeInterval: "1m",
								},
							},
							Wal: &otel.WALConfig{
								Enabled:           true,
								BufferSize:        int64(100),
								TruncateFrequency: durationpb.New(time.Second * 5),
							},
						},
					},
				},
			}
			t := otel.OTELTemplates
			aggregatorTmpl, err := t.Parse(`{{template "aggregator-config" .}}`)
			Expect(err).To(BeNil())
			for i, cfg := range validCfgs {
				var b bytes.Buffer
				err = aggregatorTmpl.Execute(&b, cfg)
				Expect(err).To(BeNil())
				Expect(strings.TrimSpace(b.String())).NotTo(BeEmpty())
				configBytes = b.Bytes()
				// cfg, err := validator.IsValid(configBytes)
				debug(fmt.Sprintf("%s-%d", step, i), configBytes)
				// Expect(err).To(Succeed())
				// Expect(cfg).NotTo(BeNil())
			}
		})
	})
})
