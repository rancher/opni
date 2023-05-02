package metrics_test

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"path"
	"time"

	"github.com/golang/snappy"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/prometheus/prompb"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/freeport"
	"github.com/rancher/opni/pkg/test/testdata"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/protoadapt"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

type agentForwarderConfig struct {
	TelemetryPort            int
	OTLPExporterEndpoint     string
	OTLPHttpExporterEndpoint string
}

var _ = Describe("Agent - OTLP metrics test", Ordered, Label("integration"), func() {
	var env *test.Environment
	var client managementv1.ManagementClient
	var nodeConfigClient node.NodeConfigurationClient
	var fingerprint string
	var mut1 *test.RemoteWriteMutator
	BeforeAll(func() {
		// start a remote write endpoint
		mut1 = test.NewRemoteWriteMutator(
			func(bytes.Buffer) { /*no-op*/ },
		)
		ctx := context.Background()
		ctxca, ca := context.WithCancel(ctx)
		DeferCleanup(ca)

		remoteWriteAddress1 := test.StartRemoteWriteServer(ctxca, mut1)

		env = &test.Environment{
			TestBin: "../../../testbin/bin",
		}
		Expect(env.Start(
			test.WithRemoteWriteEndpoints(
				remoteWriteAddress1,
			),
		)).To(Succeed())
		DeferCleanup(env.Stop)
		client = env.NewManagementClient()
		nodeConfigClient = node.NewNodeConfigurationClient(env.ManagementClientConn())

		certsInfo, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		fingerprint = certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
		Expect(fingerprint).NotTo(BeEmpty())
	})

	When("we start agent(s) with otel metrics capability", func() {
		Specify("the gateway collector should receive metrics in the expected format", func() {
			By("bootstraping the agent")
			token, err := client.CreateBootstrapToken(env.Context(), &managementv1.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(time.Minute),
			})
			Expect(err).NotTo(HaveOccurred())

			_, errC := env.StartAgent("otelmetrics", token, []string{fingerprint})
			Eventually(errC).Should(Receive(BeNil()))

			By("installing the metrics backend & capability")
			cortexOpsClient := cortexops.NewCortexOpsClient(env.ManagementClientConn())
			_, err = cortexOpsClient.ConfigureCluster(context.Background(), &cortexops.ClusterConfiguration{
				Mode: cortexops.DeploymentMode_AllInOne,
				Storage: &storagev1.StorageSpec{
					Backend: storagev1.Filesystem,
					Filesystem: &storagev1.FilesystemStorageSpec{
						Directory: path.Join(env.GetTempDirectory(), "cortex", "data"),
					},
				},
				Grafana: &cortexops.GrafanaConfig{
					Enabled: false,
				},
			})
			Expect(err).To(Succeed())
			_, err = nodeConfigClient.SetNodeConfiguration(context.Background(), &node.NodeConfigRequest{
				Node: &corev1.Reference{
					Id: "otelmetrics",
				},
				Spec: &node.MetricsCapabilitySpec{
					Otel: &node.OTELSpec{},
				},
			})
			Expect(err).To(Succeed())

			_, err = client.InstallCapability(env.Context(), &managementv1.CapabilityInstallRequest{
				Name: "metrics",
				Target: &capabilityv1.InstallRequest{
					Cluster: &corev1.Reference{Id: "otelmetrics"},
				},
			})
			Expect(err).To(Succeed())
			agent := env.GetAgent("otelmetrics")
			forwarderAddress := fmt.Sprintf("http://%s/api/agent/otel", agent.Agent.ListenAddress())
			By("setting up an external metrics forwarder")
			forwarderAgentConfig := testdata.TestData("otel/otlp-agent-forwarder.tmpl")
			forwarderTmpl, err := template.New("forwarderAgentConfig").Parse(string(forwarderAgentConfig))
			Expect(err).ToNot(HaveOccurred())
			var fb bytes.Buffer
			err = forwarderTmpl.Execute(&fb, agentForwarderConfig{
				TelemetryPort:            freeport.GetFreePort(),
				OTLPHttpExporterEndpoint: forwarderAddress,
			})
			Expect(err).ToNot(HaveOccurred())
			forwarderAgentConfig = fb.Bytes()
			env.StartEmbeddedOTELCollector(
				env.Context(),
				forwarderAgentConfig,
				path.Join(env.GetTempDirectory(), fmt.Sprintf("forwarder-%s.yaml", uuid.New().String())),
			)

			By("verifying the remote write mutator receives metrics")

			receiver := make(chan []prompb.TimeSeries)
			mut1.SetFn(func(input bytes.Buffer) {
				// prometheus exporter sends it this way
				data := input.Bytes()
				decodeData := []byte{}
				outputData, err := snappy.Decode(decodeData, data)
				if err != nil {
					return
				}
				if len(outputData) > 0 {
					decodeData = outputData
				}
				writeReq := prompb.WriteRequest{}
				err = proto.Unmarshal(decodeData, protoadapt.MessageV2Of(&writeReq))
				if err != nil {
					return
				}
				receiver <- writeReq.Timeseries
			})
			Eventually(receiver, 15*time.Second).Should(Receive(Not(HaveLen(0))))

			By("verifying received metrics received are labelled with tenant properties")
			sampleMsg := <-receiver
			Expect(sampleMsg).NotTo(HaveLen(0))
			for _, item := range sampleMsg {
				hasTenant := false
				for _, label := range item.Labels {
					if label.Name == "__tenant_id__" {
						hasTenant = true
						Expect(label.Value).To(Equal("otelmetrics"))
					}
				}
				Expect(hasTenant).To(BeTrue())
			}
		})
	})
})
