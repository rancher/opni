package session_test

import (
	"context"
	"net"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/challenges"
	"github.com/rancher/opni/pkg/auth/cluster"
	authv2 "github.com/rancher/opni/pkg/auth/cluster/v2"
	"github.com/rancher/opni/pkg/auth/session"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/keyring/ephemeral"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/testgrpc"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/samber/lo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

var _ = Describe("Session Attributes Challenge", Ordered, func() {
	var (
		goodKey1, goodKey2,
		badKey1, badKey2,
		skipKey1, skipKey2 *ephemeral.Key
	)
	serverHandler := &atomic.Pointer[func(stream testgrpc.StreamService_StreamServer) error]{}
	var dialer func(context.Context, string) (net.Conn, error)
	var dial = func(kr ...keyring.Keyring) (*grpc.ClientConn, error) {
		var clientKeyring keyring.Keyring
		if len(kr) > 0 {
			clientKeyring = kr[0]
		} else {
			clientKeyring = testKeyring
		}
		clientChallenge, err := session.NewClientChallenge(clientKeyring)
		Expect(err).NotTo(HaveOccurred())

		handler := challenges.Chained(
			testutil.Must(authv2.NewClientChallenge(clientKeyring, "foo", test.Log)),
			challenges.If(clientChallenge.HasAttributes).Then(clientChallenge),
		)

		return grpc.Dial("bufconn", grpc.WithContextDialer(dialer), grpc.WithInsecure(),
			grpc.WithStreamInterceptor(cluster.StreamClientInterceptor(handler)))
	}
	BeforeAll(func() {
		goodKey1 = ephemeral.NewKey(ephemeral.Authentication, map[string]string{
			session.AttributeLabelKey: "good1",
		})
		goodKey2 = ephemeral.NewKey(ephemeral.Authentication, map[string]string{
			session.AttributeLabelKey: "good2",
		})
		badKey1 = &ephemeral.Key{
			Usage:  ephemeral.Authentication,
			Secret: make([]byte, 33),
			Labels: map[string]string{
				session.AttributeLabelKey: "bad1",
			},
		}
		badKey2 = &ephemeral.Key{
			Usage:  "foo",
			Secret: make([]byte, 32),
			Labels: map[string]string{
				session.AttributeLabelKey: "bad2",
			},
		}
		skipKey1 = ephemeral.NewKey(ephemeral.Authentication, map[string]string{
			"foo": "skip1", // wrong key
		})
		skipKey2 = ephemeral.NewKey(ephemeral.Authentication, map[string]string{
			session.AttributeMetadataKey: "skip2", // note the wrong constant
		})

		serverHandler.Store(lo.ToPtr(func(stream testgrpc.StreamService_StreamServer) error {
			return nil
		}))

		testServer := &testgrpc.StreamServer{
			ServerHandler: func(stream testgrpc.StreamService_StreamServer) error {
				return (*serverHandler.Load())(stream)
			},
		}

		attrChallenge, err := session.NewServerChallenge(testKeyring)
		Expect(err).NotTo(HaveOccurred())

		broker := test.NewTestKeyringStoreBroker(ctrl)
		broker.KeyringStore("gateway", &corev1.Reference{Id: "foo"}).Put(context.Background(), testKeyring)

		verifier := challenges.NewKeyringVerifier(broker, test.Log)

		serverMw := challenges.Chained(
			testutil.Must(authv2.NewServerChallenge(verifier, test.Log)),
			challenges.If(session.ShouldEnableIncoming).Then(attrChallenge),
		)

		server := grpc.NewServer(grpc.Creds(insecure.NewCredentials()),
			grpc.StreamInterceptor(cluster.StreamServerInterceptor(serverMw)))

		testgrpc.RegisterStreamServiceServer(server, testServer)
		listener := bufconn.Listen(1024 * 1024)
		go server.Serve(listener)

		dialer = func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}

		DeferCleanup(listener.Close)
	})
	When("creating a new challenge handler", func() {
		When("there are no ephemeral keys in the keyring", func() {
			It("should succeed, but have no attributes", func() {
				c, err := session.NewServerChallenge(keyring.New())
				Expect(err).NotTo(HaveOccurred())
				Expect(c).NotTo(BeNil())
				Expect(c.Attributes()).To(BeEmpty())
			})
		})
		When("there are ephemeral keys in the keyring", func() {
			When("any keys are invalid", func() {
				It("should fail", func() {
					kr := keyring.New(goodKey1, badKey1, goodKey2, badKey2, skipKey1, skipKey2)
					_, err := session.NewServerChallenge(kr)
					Expect(err).To(HaveOccurred())

					kr = keyring.New(goodKey1, badKey1, goodKey1, skipKey1, skipKey2)
					_, err = session.NewServerChallenge(kr)
					Expect(err).To(HaveOccurred())

					kr = keyring.New(goodKey1, badKey1, goodKey2, skipKey1, skipKey2)
					_, err = session.NewServerChallenge(kr)
					Expect(err).To(HaveOccurred())
				})
			})
		})
		When("all keys are valid", func() {
			It("should succeed", func() {
				kr := keyring.New(goodKey1, goodKey2, skipKey1, skipKey2)
				_, err := session.NewServerChallenge(kr)
				Expect(err).NotTo(HaveOccurred())
			})
			It("should have the correct attributes", func() {
				kr := keyring.New(goodKey1, goodKey2, skipKey1, skipKey2)
				c, err := session.NewServerChallenge(kr)
				Expect(err).NotTo(HaveOccurred())
				Expect(c.Attributes()).To(ConsistOf("good1", "good2"))
			})
		})
	})
	Context("Challenge Handlers", func() {
		When("the client has no attribute keys in its keyring", func() {
			It("should have no attributes in the server stream context", func() {
				attrs := make(chan []session.Attribute, 1)
				serverHandler.Store(lo.ToPtr(func(stream testgrpc.StreamService_StreamServer) error {
					attrs <- session.StreamAuthorizedAttributes(stream.Context())
					return nil
				}))
				cc, err := dial(keyring.New(testSharedKeys))
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()

				client := testgrpc.NewStreamServiceClient(cc)
				_, err = client.Stream(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Eventually(attrs).Should(Receive(HaveLen(0)))
			})
		})
		When("the client has attribute keys in its keyring", func() {
			It("should add attributes to the server stream context", func() {
				attrs := make(chan []session.Attribute, 1)
				serverHandler.Store(lo.ToPtr(func(stream testgrpc.StreamService_StreamServer) error {
					attrs <- session.StreamAuthorizedAttributes(stream.Context())
					return nil
				}))
				cc, err := dial(testKeyring) // has 2 attribute keys
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()

				client := testgrpc.NewStreamServiceClient(cc)
				_, err = client.Stream(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Eventually(attrs).Should(Receive(HaveLen(2)))
			})
		})
		When("the client requests attributes the server does not have keys for", func() {
			It("should error", func() {
				attrs := make(chan []session.Attribute, 1)
				serverHandler.Store(lo.ToPtr(func(stream testgrpc.StreamService_StreamServer) error {
					attrs <- session.StreamAuthorizedAttributes(stream.Context())
					return nil
				}))
				cc, err := dial(testKeyring.Merge(keyring.New(goodKey1)))
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()

				client := testgrpc.NewStreamServiceClient(cc)
				_, err = client.Stream(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(attrs).NotTo(Receive())
			})
		})
		When("the client fails an authentication challenge", func() {
			It("should error", func() {
				attrs := make(chan []session.Attribute, 1)
				serverHandler.Store(lo.ToPtr(func(stream testgrpc.StreamService_StreamServer) error {
					attrs <- session.StreamAuthorizedAttributes(stream.Context())
					return nil
				}))
				// add a mismatched key to the client keyring
				badAttrKey2 := ephemeral.NewKey(ephemeral.Authentication, map[string]string{
					session.AttributeLabelKey: "example-session-attribute-2",
				})
				cc, err := dial(keyring.New(testSharedKeys, sessionAttrKey1, badAttrKey2))
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()

				client := testgrpc.NewStreamServiceClient(cc)
				_, err = client.Stream(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(attrs).NotTo(Receive())
			})
		})
	})
})
