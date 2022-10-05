package bootstrap_test

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"time"

	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jws"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	bootstrapv2 "github.com/rancher/opni/pkg/apis/bootstrap/v2"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/bootstrap"
	"github.com/rancher/opni/pkg/ecdh"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/rancher/opni/pkg/util"
)

var _ = Describe("Server V2", func() {
	var token *corev1.BootstrapToken
	var token2 *corev1.BootstrapToken
	var cert *tls.Certificate
	var client bootstrapv2.BootstrapClient
	var mockTokenStore storage.TokenStore
	var mockClusterStore storage.ClusterStore
	var mockKeyringStoreBroker storage.KeyringStoreBroker

	BeforeEach(func() {
		ctx, ca := context.WithCancel(context.Background())
		DeferCleanup(ca)
		mockTokenStore = test.NewTestTokenStore(ctx, ctrl)
		mockClusterStore = test.NewTestClusterStore(ctrl)
		mockKeyringStoreBroker = test.NewTestKeyringStoreBroker(ctrl)

		token, _ = mockTokenStore.CreateToken(context.Background(), 1*time.Hour,
			storage.WithLabels(map[string]string{"foo": "bar"}),
		)
		token2, _ = mockTokenStore.CreateToken(context.Background(), 1*time.Hour)
	})

	JustBeforeEach(func() {
		var err error
		crt, err := tls.X509KeyPair(test.TestData("self_signed_leaf.crt"), test.TestData("self_signed_leaf.key"))
		Expect(err).NotTo(HaveOccurred())
		crt.Leaf, err = x509.ParseCertificate(crt.Certificate[0])
		Expect(err).NotTo(HaveOccurred())
		cert = &crt

		srv := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
		server := bootstrap.NewServerV2(bootstrap.StorageConfig{
			TokenStore:         mockTokenStore,
			ClusterStore:       mockClusterStore,
			KeyringStoreBroker: mockKeyringStoreBroker,
		}, cert.PrivateKey.(crypto.Signer))
		bootstrapv2.RegisterBootstrapServer(srv, server)

		listener := bufconn.Listen(1024 * 1024)
		go srv.Serve(listener)

		cc, err := grpc.Dial("bufconn", grpc.WithDialer(func(s string, d time.Duration) (net.Conn, error) {
			return listener.Dial()
		}), grpc.WithInsecure())
		Expect(err).NotTo(HaveOccurred())
		client = bootstrapv2.NewBootstrapClient(cc)

		DeferCleanup(func() {
			srv.Stop()
		})
	})

	When("sending a bootstrap join request", func() {
		When("no Authorization header is given", func() {
			It("should return the correct bootstrap join response", func() {
				resp, err := client.Join(context.Background(), &bootstrapv2.BootstrapJoinRequest{})
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Signatures).To(HaveLen(2))

				rawToken, err := tokens.FromBootstrapToken(token)
				Expect(err).NotTo(HaveOccurred())
				sig, _ := rawToken.SignDetached(cert.PrivateKey)
				Expect(resp.Signatures).To(HaveKeyWithValue(rawToken.HexID(), sig))
			})
		})
		When("no tokens are available", func() {
			BeforeEach(func() {
				mockTokenStore.DeleteToken(context.Background(), token.Reference())
				mockTokenStore.DeleteToken(context.Background(), token2.Reference())
			})
			It("should return http 409", func() {
				_, err := client.Join(context.Background(), &bootstrapv2.BootstrapJoinRequest{})
				Expect(util.StatusCode(err)).To(Equal(codes.Unavailable))
			})
		})
	})
	When("sending a bootstrap auth request", func() {
		When("an Authorization header is not given", func() {
			It("should return http 401", func() {
				_, err := client.Auth(context.Background(), &bootstrapv2.BootstrapAuthRequest{})
				Expect(util.StatusCode(err)).To(Equal(codes.Unauthenticated))
			})
		})
		When("an Authorization header is given", func() {
			When("the token is valid", func() {
				When("the client does not send a valid bootstrap auth request", func() {
					It("should return http 400", func() {
						rawToken, err := tokens.FromBootstrapToken(token)
						Expect(err).NotTo(HaveOccurred())
						jsonData, err := json.Marshal(rawToken)
						Expect(err).NotTo(HaveOccurred())
						sig, err := jws.Sign(jsonData, jwa.EdDSA, cert.PrivateKey)
						ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("Authorization", "Bearer "+string(sig)))

						_, err = client.Auth(ctx, &bootstrapv2.BootstrapAuthRequest{})
						Expect(util.StatusCode(err)).To(Equal(codes.InvalidArgument))
					})
				})
				When("the client sends a bootstrap auth request", func() {
					It("should return http 200", func() {
						rawToken, err := tokens.FromBootstrapToken(token)
						Expect(err).NotTo(HaveOccurred())
						jsonData, err := json.Marshal(rawToken)
						Expect(err).NotTo(HaveOccurred())
						sig, err := jws.Sign(jsonData, jwa.EdDSA, cert.PrivateKey)
						Expect(err).NotTo(HaveOccurred())

						ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("Authorization", "Bearer "+string(sig)))
						ekp := ecdh.NewEphemeralKeyPair()
						authReq := bootstrapv2.BootstrapAuthRequest{
							ClientID:     "foo",
							ClientPubKey: ekp.PublicKey,
						}

						_, err = client.Auth(ctx, &authReq)
						Expect(err).NotTo(HaveOccurred())
						token, err := mockTokenStore.GetToken(context.Background(), rawToken.Reference())
						Expect(err).NotTo(HaveOccurred())
						Expect(token.GetMetadata().GetUsageCount()).To(Equal(int64(1)))

						clusterList, err := mockClusterStore.ListClusters(context.Background(), &corev1.LabelSelector{}, 0)
						Expect(err).NotTo(HaveOccurred())
						Expect(clusterList.Items).To(HaveLen(1))
						Expect(clusterList.Items[0].GetLabels()).To(HaveKeyWithValue("foo", "bar"))

						By("checking that the cluster's keyring was stored")
						ks := mockKeyringStoreBroker.KeyringStore("gateway", &corev1.Reference{
							Id: "foo",
						})
						kr, err := ks.Get(context.Background())
						Expect(err).NotTo(HaveOccurred())
						Expect(kr).NotTo(BeNil())
					})
				})
			})
			When("the token is invalid", func() {
				It("should return http 401", func() {
					ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("Authorization", "Bearer invalid"))
					ekp := ecdh.NewEphemeralKeyPair()
					authReq := bootstrapv2.BootstrapAuthRequest{
						ClientID:     "foo",
						ClientPubKey: ekp.PublicKey,
					}

					_, err := client.Auth(ctx, &authReq)
					Expect(util.StatusCode(err)).To(Equal(codes.PermissionDenied))
				})
			})
			When("the token is valid but expired", func() {
				BeforeEach(func() {
					mockTokenStore.DeleteToken(context.Background(), token.Reference())
				})
				It("should return http 401", func() {
					rawToken, err := tokens.FromBootstrapToken(token)
					Expect(err).NotTo(HaveOccurred())
					jsonData, err := json.Marshal(rawToken)
					Expect(err).NotTo(HaveOccurred())
					sig, err := jws.Sign(jsonData, jwa.EdDSA, cert.PrivateKey)
					Expect(err).NotTo(HaveOccurred())
					ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("Authorization", "Bearer "+string(sig)))

					ekp := ecdh.NewEphemeralKeyPair()
					authReq := bootstrapv2.BootstrapAuthRequest{
						ClientID:     "foo",
						ClientPubKey: ekp.PublicKey,
					}
					_, err = client.Auth(ctx, &authReq)
					Expect(err).To(HaveOccurred())
					Expect(util.StatusCode(err)).To(Equal(codes.PermissionDenied))
				})
			})
			When("the cluster already exists", func() {
				It("should return codes.AlreadyExists", func() {
					rawToken, err := tokens.FromBootstrapToken(token)
					Expect(err).NotTo(HaveOccurred())
					jsonData, err := json.Marshal(rawToken)
					Expect(err).NotTo(HaveOccurred())
					sig, err := jws.Sign(jsonData, jwa.EdDSA, cert.PrivateKey)
					Expect(err).NotTo(HaveOccurred())
					ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("Authorization", "Bearer "+string(sig)))

					ekp := ecdh.NewEphemeralKeyPair()
					authReq := bootstrapv2.BootstrapAuthRequest{
						ClientID:     "foo",
						ClientPubKey: ekp.PublicKey,
					}
					_, err = client.Auth(ctx, &authReq)
					Expect(err).NotTo(HaveOccurred())

					_, err = client.Auth(ctx, &authReq)
					Expect(err).To(HaveOccurred())
					Expect(util.StatusCode(err)).To(Equal(codes.AlreadyExists))
				})
			})
		})
	})
})
