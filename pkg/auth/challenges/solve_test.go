package challenges_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/challenges"
	authutil "github.com/rancher/opni/pkg/auth/util"
)

var _ = Describe("Challenge", func() {
	seen := map[string]struct{}{}
	DescribeTable("It should solve challenges with different parameters",
		func(challenge, id, clientRand string, key []byte, domain string) {
			resp := challenges.Solve(&corev1.ChallengeRequest{
				Challenge: []byte(challenge),
			}, challenges.ClientMetadata{
				IdAssertion: string(id),
				Random:      []byte(clientRand),
			}, key, domain)
			Expect(resp.Response).To(HaveLen(64))
			Expect(seen).NotTo(HaveKey(string(resp.Response)))
			seen[string(resp.Response)] = struct{}{}
		},
		Entry(nil, "challenge", "id", "rand", []byte("key"), "domain"),
		Entry(nil, "challenge2", "id", "rand", []byte("key"), "domain"),
		Entry(nil, "challenge", "id2", "rand", []byte("key"), "domain"),
		Entry(nil, "challenge", "id", "rand2", []byte("key"), "domain"),
		Entry(nil, "challenge", "id", "rand", []byte("key2"), "domain"),
		Entry(nil, "challenge", "id", "rand", []byte("key"), "domain2"),
		Entry("random data", randomString(), randomString(), randomString(), authutil.NewRandom256(), randomString()),
	)

	When("the provided key is too long", func() {
		It("should panic", func() {
			Expect(func() {
				challenges.Solve(&corev1.ChallengeRequest{
					Challenge: []byte("challenge"),
				}, challenges.ClientMetadata{
					IdAssertion: "id",
					Random:      []byte("rand"),
				}, make([]byte, 128), "domain")
			}).To(Panic())
		})
	})
})

func randomString() string {
	return string(authutil.NewRandom256())
}
