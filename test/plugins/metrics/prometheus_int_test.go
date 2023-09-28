package metrics_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/test"
)

//#region Test Setup

var _ = Describe("Gateway - Prometheus Communication Tests", Ordered, Label("integration"), func() {
	var environment *test.Environment
	var client managementv1.ManagementClient
	var fingerprint string
	BeforeAll(func() {
		environment = &test.Environment{}
		Expect(environment.Start()).To(Succeed())
		client = environment.NewManagementClient()

		opsClient := cortexops.NewCortexOpsClient(environment.ManagementClientConn())
		err := cortexops.InstallWithPreset(environment.Context(), opsClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(cortexops.WaitForReady(environment.Context(), opsClient)).To(Succeed())

		certsInfo, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		fingerprint = certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
		Expect(fingerprint).NotTo(BeEmpty())
	})

	AfterAll(func() {
		Expect(environment.Stop()).To(Succeed())
	})

	//#endregion

	//#region Happy Path Tests

	When("querying metrics from the gateway", func() {
		It("can return Prometheus metrics", func() {
			token, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(time.Minute),
			})
			Expect(err).NotTo(HaveOccurred())

			certsInfo, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			fingerprint = certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
			Expect(fingerprint).NotTo(BeEmpty())

			_, errC := environment.StartAgent("test-cluster-id", token, []string{fingerprint})
			Eventually(errC).Should(Receive(BeNil()))

			_, err = client.InstallCapability(context.Background(), &managementv1.CapabilityInstallRequest{
				Name: wellknown.CapabilityMetrics,
				Target: &capabilityv1.InstallRequest{
					Cluster: &corev1.Reference{Id: "test-cluster-id"},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			//http request to the gateway endpoint including auth header
			_, err = client.CreateBackendRole(context.Background(), &corev1.BackendRole{
				Capability: &corev1.CapabilityType{
					Name: wellknown.CapabilityMetrics,
				},
				Role: &corev1.Role{
					Id: "test-role",
					Permissions: []*corev1.PermissionItem{
						{
							Type: string(corev1.PermissionTypeCluster),
							Verbs: []*corev1.PermissionVerb{
								corev1.VerbGet(),
							},
							Ids: []string{"test-cluster-id"},
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
				Id:       "test-role-binding",
				RoleId:   "test-role",
				Subjects: []string{"user@example.com"},
				Metadata: &corev1.RoleBindingMetadata{
					Capability: lo.ToPtr(wellknown.CapabilityMetrics),
				},
			})
			Expect(err).NotTo(HaveOccurred())
			tlsConfig := environment.GatewayClientTLSConfig()

			httpClient := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: tlsConfig,
				},
			}

			var resp *http.Response
			Eventually(func() error {
				req, err := http.NewRequest("GET", environment.PrometheusAPIEndpoint()+"/labels", nil)
				Expect(err).NotTo(HaveOccurred())

				req.Header.Add("Accept", "application/json")
				req.Header.Add("Authorization", "user@example.com")
				resp, err = httpClient.Do(req)
				if err != nil {
					return err
				}
				if resp.StatusCode != http.StatusOK {
					return fmt.Errorf("unexpected status: %s", resp.Status)
				}
				return nil
			}).Should(Succeed())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/json"))
			Expect(resp.Header.Get("Results-Cache-Gen-Number")).To(BeEmpty())

			now := time.Now().Unix()
			layout := "Mon, 02 Jan 2006 15:04:05 MST"
			respTime, errT := time.Parse(layout, resp.Header.Get("Date"))
			Expect(errT).NotTo(HaveOccurred())
			Expect(respTime.Unix()).To(BeNumerically("<=", (now)))
			defer resp.Body.Close()
			b, err := ioutil.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal(`{"status":"success","data":["__tenant_id__"]}`))
		})
	})

	//#endregion

	//#region Edge Case Tests

	//TODO: Add edge case tests

	//#endregion
})
