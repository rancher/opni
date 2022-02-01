package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kralicky/opni-monitoring/pkg/core"
	"github.com/kralicky/opni-monitoring/pkg/logger"
	"github.com/kralicky/opni-monitoring/pkg/management"
	"github.com/kralicky/opni-monitoring/pkg/pkp"
	"github.com/kralicky/opni-monitoring/pkg/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fingerprintsData struct {
	TestData []fingerprintsTestData `json:"testData"`
}

type fingerprintsTestData struct {
	Cert         string             `json:"cert"`
	Fingerprints map[pkp.Alg]string `json:"fingerprints"`
}

var testFingerprints fingerprintsData

var _ = Describe("Management Test", Ordered, func() {
	var environment *test.Environment
	var client management.ManagementClient
	BeforeAll(func() {
		fmt.Println("Starting test environment")
		environment = &test.Environment{
			TestBin: "../../testbin/bin",
			Logger:  logger.New().Named("test"),
		}
		Expect(environment.Start()).To(Succeed())
		client = environment.NewManagementClient()
		Expect(json.Unmarshal(test.TestData("fingerprints.json"), &testFingerprints)).To(Succeed())
	})

	AfterAll(func() {
		fmt.Println("Stopping test environment")
		Expect(environment.Stop()).To(Succeed())
	})

	var token *core.BootstrapToken
	var fingerprint string
	It("can create a bootstrap token", func() {
		var err error
		token, err = client.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("can list all bootstrap tokens", func() {
		tokenList, err := client.ListBootstrapTokens(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		tokenInfo := tokenList.Items
		Expect(tokenInfo).To(HaveLen(1))
		for _, token := range tokenInfo {
			Expect(token.TokenID).NotTo(BeEmpty())
			Expect(token.Secret).NotTo(BeEmpty())
			Expect(token.Ttl).NotTo(BeZero())
			Expect(token.LeaseID).NotTo(BeZero())
		}
	})

	It("can revoke a bootstrap token", func() {
		_, err := client.RevokeBootstrapToken(context.Background(), token.Reference())
		Expect(err).NotTo(HaveOccurred())

		tokenList, err := client.ListBootstrapTokens(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		tokenInfo := tokenList.Items
		Expect(tokenInfo).To(BeEmpty())
	})

	It("can retrieve full certification chain information", func() {
		certsInfo, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		leaf := certsInfo.Chain[0]
		root := certsInfo.Chain[len(certsInfo.Chain)-1]

		fingerprint = root.Fingerprint
		Expect(root.Issuer).To(Equal("CN=Example Root CA"))
		Expect(root.Subject).To(Equal("CN=Example Root CA"))
		Expect(root.IsCA).To(BeTrue())
		Expect(root.Fingerprint).NotTo(BeEmpty())

		Expect(leaf.Issuer).To(Equal("CN=Example Root CA"))
		Expect(leaf.Subject).To(Equal("CN=leaf"))
		Expect(leaf.IsCA).To(BeFalse())
		Expect(leaf.Fingerprint).NotTo(BeEmpty())
	})

	When("an agent is added and there are no tokens", func() {
		It("should fail to bootstrap", func() {
			_, errC := environment.StartAgent("foo", token, []string{fingerprint})
			Eventually(errC).Should(Receive(MatchError("bootsrap error: bootstrap failed: 405 Method Not Allowed")))
		})
	})
})
