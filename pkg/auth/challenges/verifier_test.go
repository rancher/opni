package challenges_test

import (
	"context"
	"crypto/rand"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/challenges"
	authutil "github.com/rancher/opni/pkg/auth/util"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test/mock/storage"
	"github.com/rancher/opni/pkg/test/testlog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ = Describe("Verifier", Label("unit"), Ordered, func() {
	const expectedId = "test-id"
	const domain = "Verifier Test"
	var krStore storage.KeyringStore
	var broker storage.KeyringStoreBroker
	var sharedKeys *keyring.SharedKeys
	var verifier challenges.KeyringVerifier
	_ = verifier

	BeforeAll(func() {
		krStore = mock_storage.NewTestKeyringStore(ctrl, "gateway", &corev1.Reference{Id: expectedId})
		bytes := make([]byte, 64)
		rand.Read(bytes)
		sharedKeys = keyring.NewSharedKeys(bytes)
		krStore.Put(context.Background(), keyring.New(sharedKeys))
		broker = mock_storage.NewTestKeyringStoreBroker(ctrl, func(_ string, ref *corev1.Reference) storage.KeyringStore {
			if ref.GetId() == expectedId {
				return krStore
			}
			return nil
		})
		verifier = challenges.NewKeyringVerifier(broker, domain, testlog.Log)
	})

	When("preparing a pre-cached verifier", func() {
		When("the cluster is not found", func() {
			It("should return an Unauthenticated error", func() {
				challenge := authutil.NewRandom256()
				cm := challenges.ClientMetadata{
					IdAssertion: "not-found",
					Random:      authutil.NewRandom256(),
				}
				cr := &corev1.ChallengeRequest{Challenge: challenge[:]}
				v, err := verifier.Prepare(context.Background(), cm, cr)
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
				Expect(v).To(BeNil())
			})
		})
		When("an error occurs while getting the keyring", func() {
			It("should return an unavailable error", func() {
				challenge := authutil.NewRandom256()
				cm := challenges.ClientMetadata{
					IdAssertion: expectedId,
					Random:      authutil.NewRandom256(),
				}
				cr := &corev1.ChallengeRequest{Challenge: challenge[:]}
				v, err := verifier.Prepare(mock_storage.ErrContext_TestKeyringStore_Get, cm, cr)
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).To(Equal(codes.Unavailable))
				Expect(v).To(BeNil())
			})
		})
		When("the keyring is found", func() {
			It("should return a pre-cached verifier", func() {
				challenge := authutil.NewRandom256()
				cm := challenges.ClientMetadata{
					IdAssertion: expectedId,
					Random:      authutil.NewRandom256(),
				}
				cr := &corev1.ChallengeRequest{Challenge: challenge[:]}
				v, err := verifier.Prepare(context.Background(), cm, cr)
				Expect(err).NotTo(HaveOccurred())
				Expect(v).NotTo(BeNil())
			})
		})
	})

	When("verifying a challenge", func() {
		When("the challenge is correct", func() {
			It("should return the associated shared keys", func() {
				challenge := authutil.NewRandom256()
				cm := challenges.ClientMetadata{
					IdAssertion: expectedId,
					Random:      authutil.NewRandom256(),
				}
				req := &corev1.ChallengeRequest{Challenge: challenge[:]}
				v, err := verifier.Prepare(context.Background(), cm, req)
				Expect(err).NotTo(HaveOccurred())

				resp := challenges.Solve(req, cm, sharedKeys.ClientKey, domain)

				keys := v.Verify(resp)
				Expect(keys).To(Equal(sharedKeys))
			})
		})
		When("the challenge is incorrect", func() {
			It("should return nil", func() {
				challenge := authutil.NewRandom256()
				cm := challenges.ClientMetadata{
					IdAssertion: expectedId,
					Random:      authutil.NewRandom256(),
				}
				req := &corev1.ChallengeRequest{Challenge: challenge[:]}
				v, err := verifier.Prepare(context.Background(), cm, req)
				Expect(err).NotTo(HaveOccurred())

				resp := challenges.Solve(req, cm, sharedKeys.ClientKey, domain)
				resp.Response[0] ^= 0xFF

				keys := v.Verify(&corev1.ChallengeResponse{Response: resp.Response})
				Expect(keys).To(BeNil())
			})
		})
	})
})
