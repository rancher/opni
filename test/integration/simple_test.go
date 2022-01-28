package integration_test

import (
	"context"
	"fmt"
	"time"

	"github.com/kralicky/opni-monitoring/pkg/core"
	"github.com/kralicky/opni-monitoring/pkg/logger"
	"github.com/kralicky/opni-monitoring/pkg/management"
	"github.com/kralicky/opni-monitoring/pkg/test"
	"github.com/kralicky/opni-monitoring/pkg/tokens"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("Simple Test", Ordered, func() {
	var environment *test.Environment
	BeforeAll(func() {
		fmt.Println("Starting test environment")
		environment = &test.Environment{
			TestBin: "../../testbin/bin",
			Logger:  logger.New().Named("test"),
		}
		Expect(environment.Start()).To(Succeed())
	})

	AfterAll(func() {
		fmt.Println("Stopping test environment")
		Expect(environment.Stop()).To(Succeed())
	})

	var token *core.BootstrapToken
	var fingerprint string
	It("should create a bootstrap token", func() {
		mgmt := environment.NewManagementClient()
		var err error
		token, err = mgmt.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Minute),
		}, grpc.WaitForReady(true))
		Expect(err).NotTo(HaveOccurred())
		certsInfo, err := mgmt.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		fingerprint = certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
		Expect(fingerprint).NotTo(BeEmpty())
	})
	When("an agent is added", func() {
		It("should become ready", func() {
			bootstrapToken, err := tokens.FromBootstrapToken(token)
			Expect(err).NotTo(HaveOccurred())
			port := environment.StartAgent("foo", bootstrapToken.EncodeHex(), []string{fingerprint})
			promAgentPort := environment.StartPrometheus(port)
			Expect(promAgentPort).NotTo(BeZero())
		})
	})
})
