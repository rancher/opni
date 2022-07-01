package integration_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/pkp"
	"github.com/rancher/opni/pkg/test"
)

//#region Test Setup

type fingerprintsData struct {
	TestData []fingerprintsTestData `json:"testData"`
}

type fingerprintsTestData struct {
	Cert         string             `json:"cert"`
	Fingerprints map[pkp.Alg]string `json:"fingerprints"`
}

var testFingerprints fingerprintsData
var _ = Describe("Agent - Agent and Gateway Bootstrap Tests", Ordered, Label("integration"), func() {
	var environment *test.Environment
	var client managementv1.ManagementClient
	var fingerprint string
	BeforeAll(func() {
		environment = &test.Environment{
			TestBin: "../../testbin/bin",
		}
		Expect(environment.Start()).To(Succeed())
		client = environment.NewManagementClient()
		Expect(json.Unmarshal(test.TestData("fingerprints.json"), &testFingerprints)).To(Succeed())
	})

	AfterAll(func() {
		Expect(environment.Stop()).To(Succeed())
	})

	//#endregion

	//#region Happy Path Tests

	When("mulitple clusters are associated to a user", func() {
		It("can return metrics associated with that user", func() {
			token, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(time.Minute),
			})
			Expect(err).NotTo(HaveOccurred())

			certsInfo, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			fingerprint = certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
			Expect(fingerprint).NotTo(BeEmpty())

			var clusterNameList []string
			var uUClusterNameList []string
			name := "test-cluster-id-" + uuid.New().String()
			clusterNameList = append(clusterNameList, name)

			port, errC := environment.StartAgent(name, token, []string{fingerprint})
			Eventually(errC).Should(Receive(BeNil()))
			promAgentPort := environment.StartPrometheus(port)
			Expect(promAgentPort).NotTo(BeZero())

			for i := 0; i < 5; i++ {
				//Clusters that will be associated to the user
				clusterName := "test-cluster-id-" + uuid.New().String()
				_, errC := environment.StartAgent(clusterName, token, []string{fingerprint})
				Eventually(errC).Should(Receive(BeNil()))
				clusterNameList = append(clusterNameList, clusterName)
			}

			//Clusters that will not be associated to the user
			token2, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(time.Minute),
			})
			Expect(err).NotTo(HaveOccurred())

			for i := 0; i < 5; i++ {
				uUClusterName := "test-cluster-id-" + uuid.New().String()
				_, errC := environment.StartAgent(uUClusterName, token2, []string{fingerprint})
				Eventually(errC).Should(Receive(BeNil()))
				uUClusterNameList = append(uUClusterNameList, uUClusterName)
			}

			_, err = client.CreateRole(context.Background(), &corev1.Role{
				Id:         "test-role",
				ClusterIDs: clusterNameList,
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
				Id:       "test-rolebinding",
				RoleId:   "test-role",
				Subjects: []string{"user@example.com"},
			})
			Expect(err).NotTo(HaveOccurred())

			accessList, err := client.SubjectAccess(context.Background(), &corev1.SubjectAccessRequest{
				Subject: "user@example.com",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(accessList.Items).To(HaveLen(6))
			var idList []string
			for _, id := range accessList.Items {
				idList = append(idList, id.Id)
			}
			Expect(idList).To(ContainElements(clusterNameList))
			Expect(idList).NotTo(ContainElements(uUClusterNameList))

			httpClient := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
					},
				},
			}
			req, err := http.NewRequest("GET", environment.PrometheusAPIEndpoint()+"/labels", nil)
			Expect(err).NotTo(HaveOccurred())

			req.Header.Add("Content-Type", "application/json")
			req.Header.Add("Authorization", "user@example.com")

			resp, httpErr := httpClient.Do(req)
			Expect(httpErr).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			now := time.Now().Unix()
			layout := "Mon, 02 Jan 2006 15:04:05 MST"
			respTime, errT := time.Parse(layout, resp.Header.Get("Date"))
			Expect(errT).NotTo(HaveOccurred())
			Expect(respTime.Unix()).To(BeNumerically("<=", (now)))
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/json"))
			Expect(resp.Header.Get("Results-Cache-Gen-Number")).To(BeEmpty())

			defer resp.Body.Close()
			b, err := ioutil.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal(`{"status":"success","data":["__tenant_id__"]}`))

			//
			invReq, err := http.NewRequest("GET", environment.PrometheusAPIEndpoint()+"/labels", nil)
			Expect(err).NotTo(HaveOccurred())

			invReq.Header.Add("Content-Type", "application/json")
			invReq.Header.Add("Authorization", "invaliduser@example.com")

			invResp, httpErr := httpClient.Do(req)
			Expect(httpErr).NotTo(HaveOccurred())
			Expect(invResp.StatusCode).To(Equal(http.StatusOK))
			Expect(invResp.Header.Get("Content-Type")).To(Equal("application/json"))
			Expect(invResp.Header.Get("Results-Cache-Gen-Number")).To(BeEmpty())

			defer invResp.Body.Close()
			b2, err := ioutil.ReadAll(invResp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b2)).To(Equal(`{"status":"success","data":["__tenant_id__"]}`))

		})
	})

	//#endregion

	//#region Edge Case Tests

	//TODO: Add edge case tests

	//#endregion
})
