package slo_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/rules"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util/notifier"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("Converting ServiceLevelObjective Messages to Prometheus Rules", Ordered, Label(test.Unit, test.Slow), func() {

	// var k8sClient client.Client
	var finder notifier.Finder[rules.RuleGroup]
	BeforeAll(func() {
		env := test.Environment{
			TestBin: "../../../../testbin/bin",
		}
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop)

		client := env.NewManagementClient()
		token, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())
		info, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		p, _ := env.StartAgent("agent", token, []string{info.Chain[len(info.Chain)-1].Fingerprint})
		env.StartPrometheus(p)
	})

	It("should initially find no groups", func() {
		groups, err := finder.Find(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(groups).To(BeEmpty())
	})

	When("A an ServiceLevelObjective message is given", func() {
		It("should validate proper input", func() {

		})
		It("should reject improper input", func() {

		})
	})
	When("We convert a ServiceLevelObjective to OpenSLO", func() {
		It("Should create a valid OpenSLO spec", func() {

		})
	})

	When("We convert an OpenSLO to Sloth IR", func() {
		It("should create a valid Sloth IR", func() {

		})
	})

	When("We convert Sloth IR to prometheus rules", func() {
		It("Should create valid prometheus rules", func() {

		})
	})

})
