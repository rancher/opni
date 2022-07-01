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

	bootstrapv1 "github.com/rancher/opni/pkg/apis/bootstrap/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/bootstrap"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/ecdh"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/rancher/opni/pkg/util"
)

type testCapBackend struct {
	Name       string
	CanInstall bool
}

var _ = Describe("Server", Label("slow"), func() {
	var token *corev1.BootstrapToken
	var token2 *corev1.BootstrapToken
	var cert *tls.Certificate
	var client bootstrapv1.BootstrapClient
	var mockTokenStore storage.TokenStore
	var mockClusterStore storage.ClusterStore
	var mockKeyringStoreBroker storage.KeyringStoreBroker
	var testCapBackends []*test.CapabilityInfo

	BeforeEach(func() {
		testCapBackends = append(testCapBackends, &test.CapabilityInfo{
			Name:       "test",
			CanInstall: true,
		})
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
	AfterEach(func() {
		testCapBackends = []*test.CapabilityInfo{}
	})
	JustBeforeEach(func() {
		var err error
		crt, err := tls.X509KeyPair(test.TestData("self_signed_leaf.crt"), test.TestData("self_signed_leaf.key"))
		Expect(err).NotTo(HaveOccurred())
		crt.Leaf, err = x509.ParseCertificate(crt.Certificate[0])
		Expect(err).NotTo(HaveOccurred())
		cert = &crt

		lg := test.Log
		capBackendStore := capabilities.NewBackendStore(capabilities.ServerInstallerTemplateSpec{}, lg)
		for _, backend := range testCapBackends {
			capBackendStore.Add(backend.Name, test.NewTestCapabilityBackend(ctrl, backend))
		}

		srv := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
		server := bootstrap.NewServer(bootstrap.StorageConfig{
			TokenStore:         mockTokenStore,
			ClusterStore:       mockClusterStore,
			KeyringStoreBroker: mockKeyringStoreBroker,
		}, cert.PrivateKey.(crypto.Signer), capBackendStore)
		bootstrapv1.RegisterBootstrapServer(srv, server)

		listener := bufconn.Listen(1024 * 1024)
		go srv.Serve(listener)

		cc, err := grpc.Dial("bufconn", grpc.WithDialer(func(s string, d time.Duration) (net.Conn, error) {
			return listener.Dial()
		}), grpc.WithInsecure())
		Expect(err).NotTo(HaveOccurred())
		client = bootstrapv1.NewBootstrapClient(cc)

		DeferCleanup(func() {
			srv.Stop()
		})
	})

	When("sending a bootstrap join request", func() {
		When("no Authorization header is given", func() {
			It("should return the correct bootstrap join response", func() {
				resp, err := client.Join(context.Background(), &bootstrapv1.BootstrapJoinRequest{})
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
				_, err := client.Join(context.Background(), &bootstrapv1.BootstrapJoinRequest{})
				Expect(util.StatusCode(err)).To(Equal(codes.Unavailable))
			})
		})
	})
	When("sending a bootstrap auth request", func() {
		When("an Authorization header is not given", func() {
			It("should return http 401", func() {
				_, err := client.Auth(context.Background(), &bootstrapv1.BootstrapAuthRequest{})
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

						_, err = client.Auth(ctx, &bootstrapv1.BootstrapAuthRequest{})
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
						authReq := bootstrapv1.BootstrapAuthRequest{
							Capability:   "test",
							ClientID:     "foo",
							ClientPubKey: ekp.PublicKey,
						}

						_, err = client.Auth(ctx, &authReq)
						Expect(err).NotTo(HaveOccurred())
						token, err := mockTokenStore.GetToken(context.Background(), rawToken.Reference())
						Expect(err).NotTo(HaveOccurred())
						Expect(token.GetMetadata().GetUsageCount()).To(Equal(int64(1)))
						Expect(token.GetMetadata().GetCapabilities()).To(ContainElement(BeEquivalentTo(&corev1.TokenCapability{
							Type: string(capabilities.JoinExistingCluster),
							Reference: &corev1.Reference{
								Id: "foo",
							},
						})))
						clusterList, err := mockClusterStore.ListClusters(context.Background(), &corev1.LabelSelector{}, 0)
						Expect(err).NotTo(HaveOccurred())
						Expect(clusterList.Items).To(HaveLen(1))
						Expect(clusterList.Items[0].GetLabels()).To(HaveKeyWithValue("foo", "bar"))

						By("checking that the cluster's keyring was stored")
						ks, err := mockKeyringStoreBroker.KeyringStore("gateway", &corev1.Reference{
							Id: "foo",
						})
						Expect(err).NotTo(HaveOccurred())
						kr, err := ks.Get(context.Background())
						Expect(err).NotTo(HaveOccurred())
						Expect(kr).NotTo(BeNil())

						By("checking that the capability was added to the cluster")
						cluster, err := mockClusterStore.GetCluster(context.Background(), &corev1.Reference{
							Id: "foo",
						})
						Expect(err).NotTo(HaveOccurred())
						Expect(cluster.GetCapabilities()).To(ContainElement(BeEquivalentTo(&corev1.ClusterCapability{
							Name: "test",
						})))
					})
				})
			})
			When("the token is invalid", func() {
				It("should return http 401", func() {
					ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("Authorization", "Bearer invalid"))
					ekp := ecdh.NewEphemeralKeyPair()
					authReq := bootstrapv1.BootstrapAuthRequest{
						Capability:   "test",
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
					authReq := bootstrapv1.BootstrapAuthRequest{
						ClientID:     "foo",
						ClientPubKey: ekp.PublicKey,
						Capability:   "test",
					}
					_, err = client.Auth(ctx, &authReq)
					Expect(err).To(HaveOccurred())
					Expect(util.StatusCode(err)).To(Equal(codes.PermissionDenied))
				})
			})
			When("joining with an additional capability", func() {
				var ekp ecdh.EphemeralKeyPair
				var newCtx func() context.Context
				JustBeforeEach(func() {
					rawToken, err := tokens.FromBootstrapToken(token)
					Expect(err).NotTo(HaveOccurred())
					jsonData, err := json.Marshal(rawToken)
					Expect(err).NotTo(HaveOccurred())
					sig, err := jws.Sign(jsonData, jwa.EdDSA, cert.PrivateKey)
					Expect(err).NotTo(HaveOccurred())
					newCtx = func() context.Context {
						return metadata.NewOutgoingContext(context.Background(), metadata.Pairs("Authorization", "Bearer "+string(sig)))
					}
					ekp = ecdh.NewEphemeralKeyPair()

					authReq := bootstrapv1.BootstrapAuthRequest{
						Capability:   "test",
						ClientID:     "foo",
						ClientPubKey: ekp.PublicKey,
					}

					ctx := newCtx()
					_, err = client.Auth(ctx, &authReq)
					Expect(err).NotTo(HaveOccurred())
				})
				When("the requested capability already exists", func() {
					It("should return http 409", func() {
						authReq := bootstrapv1.BootstrapAuthRequest{
							Capability:   "test",
							ClientID:     "foo",
							ClientPubKey: ekp.PublicKey,
						}

						ctx := newCtx()
						_, err := client.Auth(ctx, &authReq)
						Expect(err).To(HaveOccurred())
						Expect(util.StatusCode(err)).To(Equal(codes.AlreadyExists))
					})
				})
				When("the requested capability does not yet exist", func() {
					BeforeEach(func() {
						testCapBackends = append(testCapBackends,
							&test.CapabilityInfo{
								Name:       "test2",
								CanInstall: true,
							},
							&test.CapabilityInfo{
								Name:       "test3",
								CanInstall: false,
							},
						)
					})
					When("a backend for the capability does not exist", func() {
						It("should return http 404", func() {
							authReq := bootstrapv1.BootstrapAuthRequest{
								Capability:   "test1.5",
								ClientID:     "foo",
								ClientPubKey: ekp.PublicKey,
							}

							req := newCtx()
							_, err := client.Auth(req, &authReq)
							Expect(err).To(HaveOccurred())
							Expect(util.StatusCode(err)).To(Equal(codes.NotFound))
						})
					})
					When("a backend for the capability exists", func() {
						It("should succeed", func() {
							authReq := bootstrapv1.BootstrapAuthRequest{
								Capability:   "test2",
								ClientID:     "foo",
								ClientPubKey: ekp.PublicKey,
							}

							req := newCtx()
							_, err := client.Auth(req, &authReq)

							Expect(err).NotTo(HaveOccurred())
						})
					})
					When("the capability cannot be installed", func() {
						It("should return http 503", func() {
							authReq := bootstrapv1.BootstrapAuthRequest{
								Capability:   "test3",
								ClientID:     "foo",
								ClientPubKey: ekp.PublicKey,
							}

							req := newCtx()
							_, err := client.Auth(req, &authReq)
							Expect(err).To(HaveOccurred())
							Expect(util.StatusCode(err)).To(Equal(codes.Unavailable))
						})
					})
					When("the token used does not have the correct join capability", func() {
						It("should return http 401", func() {
							rawToken2, err := tokens.FromBootstrapToken(token2)
							Expect(err).NotTo(HaveOccurred())
							jsonData, err := json.Marshal(rawToken2)
							Expect(err).NotTo(HaveOccurred())
							sig, err := jws.Sign(jsonData, jwa.EdDSA, cert.PrivateKey)
							Expect(err).NotTo(HaveOccurred())

							ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("Authorization", "Bearer "+string(sig)))

							authReq := bootstrapv1.BootstrapAuthRequest{
								Capability:   "test2",
								ClientID:     "foo",
								ClientPubKey: ekp.PublicKey,
							}
							_, err = client.Auth(ctx, &authReq)
							Expect(err).To(HaveOccurred())
							Expect(util.StatusCode(err)).To(Equal(codes.PermissionDenied))
						})
					})
				})
			})
		})
	})
})
