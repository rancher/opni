package integration

import (
	"context"
	"errors"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/test"
)

var _ = Describe("Service Recovery Tests", Ordered, Label("integration"), func() {
	var environment *test.Environment
	It("should handle services being unhealthy on startup", func() {
		environment = &test.Environment{
			TestBin: "../../../testbin/bin",
		}
		delay := make(chan struct{})
		Expect(environment.Start(
			test.WithDelayStartEtcd(delay),
			test.WithDelayStartCortex(delay),
		)).To(Succeed())
		DeferCleanup(environment.Stop)

		go func() {
			defer GinkgoRecover()
			client := environment.NewManagementClient()
			certsInfo, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			fingerprint := certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
			Expect(fingerprint).NotTo(BeEmpty())
			token, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(time.Hour),
			})
			port, errC := environment.StartAgent("agent1", token, []string{fingerprint})
			environment.StartPrometheus(port)

			select {
			case err := <-errC:
				Expect(err).NotTo(HaveOccurred())
			}
		}()

		adminClient := environment.NewCortexAdminClient()
		testAgentOk := func(timeout time.Duration) error {
			ctx, ca := context.WithTimeout(context.Background(), timeout)
			defer ca()
			stats, err := adminClient.AllUserStats(ctx, &emptypb.Empty{})
			if err != nil {
				return err
			}
			for _, stat := range stats.Items {
				if stat.UserID == "agent1" {
					if stat.NumSeries > 0 {
						return nil
					}
				}
			}
			return errors.New("agent1 didn't post any metrics yet")
		}
		Consistently(func() error {
			return testAgentOk(250 * time.Millisecond)
		}, 20*time.Second, 250*time.Millisecond).ShouldNot(Succeed())
		close(delay)
		Eventually(func() error {
			return testAgentOk(250 * time.Millisecond)
		}, 1*time.Minute, 250*time.Millisecond).Should(Succeed())
		Consistently(func() error {
			return testAgentOk(250 * time.Millisecond)
		}, 2*time.Second, 250*time.Millisecond).Should(Succeed())
	})
})
