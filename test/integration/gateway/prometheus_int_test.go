package integration_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/test"
)

//#region Test Setup

var _ = Describe("Gateway - Prometheus Communication Tests", Ordered, func() {
	var environment *test.Environment
	var client management.ManagementClient
	var fingerprint string
	BeforeAll(func() {
		environment = &test.Environment{
			TestBin: "../../../testbin/bin",
			Logger:  logger.New().Named("test"),
		}
		Expect(environment.Start()).To(Succeed())
		client = environment.NewManagementClient()
		Expect(json.Unmarshal(test.TestData("fingerprints.json"), &testFingerprints)).To(Succeed())

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

	//TODO: Joe to look into how to make this work
	When("querying metrics from the gateway", func() {
		It("can return Prometheus metrics", func() {
			token, err := client.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(time.Minute),
			})
			Expect(err).NotTo(HaveOccurred())

			certsInfo, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			fingerprint = certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
			Expect(fingerprint).NotTo(BeEmpty())

			port, errC := environment.StartAgent("test-cluster-id", token, []string{fingerprint})
			promAgentPort := environment.StartPrometheus(port)
			Expect(promAgentPort).NotTo(BeZero())
			Consistently(errC).ShouldNot(Receive())

			//http request to the gateway endpoint including auth header
			_, err = client.CreateRole(context.Background(), &core.Role{
				Id:         "test-role",
				ClusterIDs: []string{"test-cluster-id"},
			})
			Expect(err).NotTo(HaveOccurred())
			_, err = client.CreateRoleBinding(context.Background(), &core.RoleBinding{
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

			req.Header.Add("Content-Type", "application/json")
			req.Header.Add("Authorization", "user@example.com")

			resp, httpErr := httpClient.Do(req)
			Expect(httpErr).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			defer resp.Body.Close()
			b, err := ioutil.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			fmt.Println(string(b))
		})
	})

	//#endregion

	//#region Edge Case Tests

	//TODO: Add edge case tests

	//#endregion
})
