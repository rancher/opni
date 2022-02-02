package management_test

import (
	context "context"
	"time"

	"github.com/kralicky/opni-monitoring/pkg/management"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("Tokens", Ordered, func() {
	var tv *testVars
	BeforeAll(setupManagementServer(&tv))

	It("should initially have no tokens", func() {
		tokens, err := tv.client.ListBootstrapTokens(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		Expect(tokens.Items).To(BeEmpty())
	})

	It("should create bootstrap tokens", func() {
		ids := map[string]struct{}{}
		secrets := map[string]struct{}{}
		leaseIds := map[int64]struct{}{}
		for i := 0; i < 100; i++ {
			token, err := tv.client.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(time.Minute),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(token.TokenID).NotTo(BeEmpty())
			Expect(token.Secret).NotTo(BeEmpty())
			Expect(token.LeaseID).NotTo(BeZero())
			Expect(token.Ttl).To(BeNumerically("~", time.Minute, time.Second))

			Expect(ids).NotTo(HaveKey(token.TokenID))
			Expect(secrets).NotTo(HaveKey(token.Secret))
			Expect(leaseIds).NotTo(HaveKey(token.LeaseID))
			ids[token.TokenID] = struct{}{}
			secrets[token.Secret] = struct{}{}
			leaseIds[token.LeaseID] = struct{}{}
		}
	})
	It("should list bootstrap tokens", func() {
		tokens, err := tv.client.ListBootstrapTokens(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		Expect(tokens.Items).To(HaveLen(100))
	})
	It("should revoke bootstrap tokens", func() {
		tokens, err := tv.client.ListBootstrapTokens(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		for i, token := range tokens.Items {
			_, err := tv.client.RevokeBootstrapToken(context.Background(), token.Reference())
			Expect(err).NotTo(HaveOccurred())

			tokens, err = tv.client.ListBootstrapTokens(context.Background(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(tokens.Items).To(HaveLen(100 - i - 1))
		}
	})
})
