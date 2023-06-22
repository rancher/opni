package integration_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/testruntime"
	"github.com/rancher/opni/pkg/test/testutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("Gateway - Gateway rate limit tests", Ordered, testruntime.EnableIfCI[FlakeAttempts](5), Label("integration", "slow"), func() {
	var environment *test.Environment
	var client managementv1.ManagementClient
	var fingerprint string
	BeforeAll(func() {
		environment = &test.Environment{}
		Expect(environment.Start(test.WithStorageBackend(v1beta1.StorageTypeEtcd))).To(Succeed())
		client = environment.NewManagementClient()

		certsInfo, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		fingerprint = certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
		Expect(fingerprint).NotTo(BeEmpty())

		DeferCleanup(environment.Stop)
	})
	When("connecting many agents to the gateway", func() {
		It("should fail after the rate limit is exceeded", func() {
			errC := make(chan error)
			for i := 0; i < 55; i++ {
				go func() {
					_, connErrC := environment.NewStreamConnection([]string{fingerprint})
					for err := range connErrC {
						errC <- err
					}
				}()
			}
			Eventually(errC).WithPolling(5 * time.Millisecond).Should(Receive(And(
				Not(BeNil()),
				MatchError(ContainSubstring("is unable to handle the request, please retry later")),
				testutil.MatchStatusCode(codes.ResourceExhausted),
			)))
		})
	})
})
