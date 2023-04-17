package metrics_test

import (
	"context"
	"crypto/tls"
	"io/ioutil"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
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
		_, err := opsClient.ConfigureCluster(context.Background(), &cortexops.ClusterConfiguration{
			Mode: cortexops.DeploymentMode_AllInOne,
			Storage: &storagev1.StorageSpec{
				Backend: storagev1.Filesystem,
			},
		})
		Expect(err).NotTo(HaveOccurred())

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
			promAgentPort := environment.StartPrometheus("test-cluster-id")
			Expect(promAgentPort).NotTo(BeZero())

			//http request to the gateway endpoint including auth header
			_, err = client.CreateRole(context.Background(), &corev1.Role{
				Id:         "test-role",
				ClusterIDs: []string{"test-cluster-id"},
			})
			Expect(err).NotTo(HaveOccurred())
			_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
				Id:       "test-role-binding",
				RoleId:   "test-role",
				Subjects: []string{"user@example.com"},
			})
			Expect(err).NotTo(HaveOccurred())

			httpClient := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
					},
				},
			}
			req, err := http.NewRequest("GET", environment.PrometheusAPIEndpoint()+"/labels", nil)
			Expect(err).NotTo(HaveOccurred())

			req.Header.Add("Accept", "application/json")
			req.Header.Add("Authorization", "user@example.com")

			resp, httpErr := httpClient.Do(req)
			Expect(httpErr).NotTo(HaveOccurred())
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