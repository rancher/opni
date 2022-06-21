package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/test"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("Agent - Remote Write Tests", Ordered, Label("integration"), func() {
	var environment *test.Environment
	var client managementv1.ManagementClient
	var fingerprint string
	BeforeAll(func() {
		environment = &test.Environment{
			TestBin: "../../../testbin/bin",
		}
		Expect(environment.Start()).To(Succeed())
		client = environment.NewManagementClient()
		Expect(json.Unmarshal(test.TestData("fingerprints.json"), &testFingerprints)).To(Succeed())

		certsInfo, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		fingerprint = certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
		Expect(fingerprint).NotTo(BeEmpty())
	})

	When("the agent starts", func() {
		It("should connect to the gateway", func() {
			token, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(time.Minute),
			})
			Expect(err).NotTo(HaveOccurred())

			port, _ := environment.StartAgent("agent1", token, []string{fingerprint})

			Eventually(func() error {
				hs, err := client.GetClusterHealthStatus(context.Background(), &v1.Reference{
					Id: "agent1",
				})
				if err != nil {
					return err
				}
				if !hs.GetStatus().GetConnected() {
					return fmt.Errorf("not connected")
				}
				conds := hs.GetHealth().GetConditions()
				if len(conds) == 1 && conds[0] == "Remote Write Pending" {
					return nil
				}
				return fmt.Errorf("waiting for remote write pending condition")
			}, 30*time.Second, 100*time.Millisecond).Should(Succeed())

			environment.StartPrometheus(port)

			Eventually(func() error {
				hs, err := client.GetClusterHealthStatus(context.Background(), &v1.Reference{
					Id: "agent1",
				})
				if err != nil {
					return err
				}
				if !hs.GetStatus().GetConnected() {
					return fmt.Errorf("not connected")
				}
				if !hs.GetHealth().GetReady() {
					return fmt.Errorf("not ready")
				}
				return nil
			}, 30*time.Second, 100*time.Millisecond).Should(Succeed())
		})
	})
})
