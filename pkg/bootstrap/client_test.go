package bootstrap_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"runtime"
	"strconv"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mock_v1 "github.com/rancher/opni/pkg/test/mock/capability"
	mock_ident "github.com/rancher/opni/pkg/test/mock/ident"
	mock_storage "github.com/rancher/opni/pkg/test/mock/storage"
	"github.com/rancher/opni/pkg/test/testdata"
	"github.com/rancher/opni/pkg/test/testlog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"

	bootstrapv1 "github.com/rancher/opni/pkg/apis/bootstrap/v1"
	v1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/bootstrap"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/pkp"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/rancher/opni/pkg/trust"
	"github.com/rancher/opni/pkg/util"
)

type errProvider struct{}

func (p *errProvider) UniqueIdentifier(context.Context) (string, error) {
	return "", errors.New("test")
}

func pkpTrustStrategy(cert *x509.Certificate) trust.Strategy {
	conf := trust.StrategyConfig{
		PKP: &trust.PKPConfig{
			Pins: trust.NewPinSource([]*pkp.PublicKeyPin{pkp.NewSha256(cert)}),
		},
	}
	return util.Must(conf.Build())
}

var _ = Describe("Client", Ordered, Label("slow"), func() {
	var fooIdent ident.Provider
	var cert *tls.Certificate
	var store storage.Backend
	var endpoint string

	BeforeAll(func() {
		if runtime.GOOS != "linux" {
			Skip("skipping test on non-linux OS")
		}
		fooIdent = mock_ident.NewTestIdentProvider(ctrl, "foo")
		var err error
		crt, err := tls.X509KeyPair(testdata.TestData("self_signed_leaf.crt"), testdata.TestData("self_signed_leaf.key"))
		Expect(err).NotTo(HaveOccurred())
		crt.Leaf, err = x509.ParseCertificate(crt.Certificate[0])
		Expect(err).NotTo(HaveOccurred())
		cert = &crt
		store = mock_storage.NewTestStorageBackend(context.Background(), ctrl)
		capBackendStore := capabilities.NewBackendStore(capabilities.ServerInstallerTemplateSpec{}, testlog.Log)
		for _, backend := range []*mock_v1.CapabilityInfo{
			{
				Name:       "test",
				CanInstall: true,
				Storage:    store,
			},
			{
				Name:       "test1",
				CanInstall: true,
				Storage:    store,
			},
			{
				Name:       "test2",
				CanInstall: true,
				Storage:    store,
			},
			{
				Name:       "test3",
				CanInstall: true,
				Storage:    store,
			},
			{
				Name:       "test4",
				CanInstall: true,
				Storage:    store,
			},
			{
				Name:       "test5",
				CanInstall: true,
				Storage:    store,
			},
		} {
			capBackendStore.Add(backend.Name, mock_v1.NewTestCapabilityBackend(ctrl, backend))
		}

		srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{*cert},
		})))

		server := bootstrap.NewServer(store, cert.PrivateKey.(crypto.Signer), capBackendStore)
		bootstrapv1.RegisterBootstrapServer(srv, server)

		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		endpoint = listener.Addr().String()

		go srv.Serve(listener)

		DeferCleanup(func() {
			srv.Stop()
		})
	})
	It("should bootstrap with the server", func() {
		token, _ := store.CreateToken(context.Background(), 1*time.Minute)
		cc := bootstrap.ClientConfig{
			Token:         testutil.Must(tokens.FromBootstrapToken(token)),
			Endpoint:      endpoint,
			TrustStrategy: pkpTrustStrategy(cert.Leaf),
			Capability:    "test",
		}

		_, err := cc.Bootstrap(context.Background(), fooIdent)
		Expect(err).NotTo(HaveOccurred())
	})
	When("bootstrapping multiple capabilities", func() {
		It("should save keyrings correctly", func() {
			By("bootstrapping two capabilities")
			token, _ := store.CreateToken(context.Background(), 1*time.Minute)
			testIdent := mock_ident.NewTestIdentProvider(ctrl, uuid.NewString())
			cc := bootstrap.ClientConfig{
				Endpoint:      endpoint,
				TrustStrategy: pkpTrustStrategy(cert.Leaf),
				Capability:    "test",
				Token:         testutil.Must(tokens.FromBootstrapToken(token)),
			}
			kr1, err := cc.Bootstrap(context.Background(), testIdent)
			Expect(err).NotTo(HaveOccurred())
			Expect(kr1).NotTo(BeNil())
			cc2 := bootstrap.ClientConfig{
				Endpoint:      endpoint,
				TrustStrategy: pkpTrustStrategy(cert.Leaf),
				Capability:    "test2",
				Token:         testutil.Must(tokens.FromBootstrapToken(token)),
			}
			kr2, err := cc2.Bootstrap(context.Background(), testIdent)
			Expect(kr2).NotTo(BeNil())
			Expect(err).NotTo(HaveOccurred())

			By("verifying that each client keyring contains one set of shared keys")
			var sk1, sk2 *keyring.SharedKeys
			kr1.Try(func(key *keyring.SharedKeys) {
				Expect(sk1).To(BeNil())
				sk1 = key
			})
			kr2.Try(func(key *keyring.SharedKeys) {
				Expect(sk2).To(BeNil())
				sk2 = key
			})
			Expect(sk1).NotTo(BeNil())
			Expect(sk2).NotTo(BeNil())
			Expect(sk1).NotTo(Equal(sk2))

			By("verifying that the server keyring contains both sets of keys")
			serverKrStore := store.KeyringStore("gateway", &v1.Reference{
				Id: testutil.Must(testIdent.UniqueIdentifier(context.Background())),
			})
			serverKr, err := serverKrStore.Get(context.Background())
			Expect(err).NotTo(HaveOccurred())
			allKeys := []*keyring.SharedKeys{}
			serverKr.Try(func(key *keyring.SharedKeys) {
				allKeys = append(allKeys, key)
			})
			Expect(allKeys).To(HaveLen(2))
			if bytes.Equal(allKeys[0].ClientKey, sk1.ClientKey) && bytes.Equal(allKeys[0].ServerKey, sk1.ServerKey) {
				Expect(bytes.Equal(allKeys[1].ClientKey, sk2.ClientKey)).To(BeTrue())
				Expect(bytes.Equal(allKeys[1].ServerKey, sk2.ServerKey)).To(BeTrue())
			} else if bytes.Equal(allKeys[0].ClientKey, sk2.ClientKey) && bytes.Equal(allKeys[0].ServerKey, sk2.ServerKey) {
				Expect(bytes.Equal(allKeys[1].ClientKey, sk1.ClientKey)).To(BeTrue())
				Expect(bytes.Equal(allKeys[1].ServerKey, sk1.ServerKey)).To(BeTrue())
			} else {
				Fail("keyrings do not match")
			}
		})
		When("multiple capabilities bootstrap simultaneously", func() {
			var testIdent ident.Provider
			BeforeEach(func() {
				testIdent = mock_ident.NewTestIdentProvider(ctrl, uuid.NewString())
			})
			validateKeyrings := func(successes []keyring.Keyring) {
				serverKrStore := store.KeyringStore("gateway", &v1.Reference{
					Id: testutil.Must(testIdent.UniqueIdentifier(context.Background())),
				})
				serverKr, err := serverKrStore.Get(context.Background())
				Expect(err).NotTo(HaveOccurred())
				var serverSharedKeys []*keyring.SharedKeys
				serverKr.Try(func(key *keyring.SharedKeys) {
					serverSharedKeys = append(serverSharedKeys, key)
				})

				var clientSharedKeys []*keyring.SharedKeys
				for _, successKr := range successes {
					var shared *keyring.SharedKeys
					successKr.Try(func(key *keyring.SharedKeys) {
						Expect(shared).To(BeNil())
						shared = key
					})
					Expect(shared).NotTo(BeNil())
					clientSharedKeys = append(clientSharedKeys, shared)
				}

				Expect(len(clientSharedKeys)).To(Equal(len(successes)),
					"the number of successful bootstraps does not match the number of keys in the server keyring")

				By("verifying that the server and client keyring shared keys are equal")
				for _, clientSharedKey := range clientSharedKeys {
					found := false
					for _, serverSharedKey := range serverSharedKeys {
						if bytes.Equal(clientSharedKey.ClientKey, serverSharedKey.ClientKey) &&
							bytes.Equal(clientSharedKey.ServerKey, serverSharedKey.ServerKey) {
							found = true
							break
						}
					}
					Expect(found).To(BeTrue(),
						"client bootstrapped successfully but its key was not found in the server's keyring")
				}
			}
			When("different tokens are used", func() {
				It("should only allow one capability to successfully bootstrap", func() {
					responses := make(chan struct {
						kr  keyring.Keyring
						err error
					}, 5)
					wait := make(chan struct{})
					By("bootstrapping several capabilities at the same time using different tokens")
					for i := 0; i < 5; i++ {
						i := i
						go func() {
							token, err := store.CreateToken(context.Background(), 1*time.Minute)
							Expect(err).NotTo(HaveOccurred())
							cc := bootstrap.ClientConfig{
								Endpoint:      endpoint,
								TrustStrategy: pkpTrustStrategy(cert.Leaf),
								Capability:    "test" + strconv.Itoa(i+1),
								Token:         testutil.Must(tokens.FromBootstrapToken(token)),
							}
							<-wait
							kr, err := cc.Bootstrap(context.Background(), testIdent)
							responses <- struct {
								kr  keyring.Keyring
								err error
							}{kr, err}
						}()
					}
					close(wait)
					By("ensuring only one cluster succeeded, and others were denied")
					var successes []keyring.Keyring
					for i := 0; i < 5; i++ {
						select {
						case <-time.After(10 * time.Second):
							Fail("timed out waiting for bootstrap")
						case resp := <-responses:
							if resp.err == nil {
								successes = append(successes, resp.kr)
							} else {
								Expect(util.StatusCode(resp.err)).To(Equal(codes.PermissionDenied))
							}
						}
					}

					Expect(successes).To(HaveLen(1))
					validateKeyrings(successes)
				})
			})
			When("the same token is used", func() {
				It("should correctly bootstrap all capabilities", func() {
					By("creating a token")
					token, err := store.CreateToken(context.Background(), 1*time.Minute)
					By("bootstrapping several capabilities at the same time using the token")
					responses := make(chan struct {
						kr  keyring.Keyring
						err error
					}, 5)
					wait := make(chan struct{})
					for i := 0; i < 5; i++ {
						i := i
						go func() {
							Expect(err).NotTo(HaveOccurred())
							cc := bootstrap.ClientConfig{
								Endpoint:      endpoint,
								TrustStrategy: pkpTrustStrategy(cert.Leaf),
								Capability:    "test" + strconv.Itoa(i+1),
								Token:         testutil.Must(tokens.FromBootstrapToken(token)),
							}
							<-wait
							var kr keyring.Keyring
							var err error
							// if 5 capabilities bootstrap at the same time, it should
							// require at most 5 requests each for all of them to succeed
							for j := 0; j < 5; j++ {
								kr, err = cc.Bootstrap(context.Background(), testIdent)
								if err == nil {
									break
								}
								Expect(util.StatusCode(err)).To(Equal(codes.PermissionDenied))
							}
							responses <- struct {
								kr  keyring.Keyring
								err error
							}{kr, err}
						}()
					}
					close(wait)
					By("ensuring that all bootstraps succeeded")
					var successes []keyring.Keyring
					for i := 0; i < 5; i++ {
						select {
						case <-time.After(10 * time.Second):
							Fail("timed out waiting for bootstrap")
						case resp := <-responses:
							Expect(resp.err).NotTo(HaveOccurred())
							successes = append(successes, resp.kr)
						}
					}
					Expect(successes).To(HaveLen(5))
					validateKeyrings(successes)
				})
			})
		})
	})
	Context("error handling", func() {
		When("no token is given", func() {
			It("should error", func() {
				cc := bootstrap.ClientConfig{}
				kr, err := cc.Bootstrap(context.Background(), fooIdent)
				Expect(kr).To(BeNil())
				Expect(err).To(MatchError(bootstrap.ErrNoToken))
			})
		})
		When("an invalid endpoint is given", func() {
			It("should error", func() {
				token, _ := store.CreateToken(context.Background(), 1*time.Minute)
				cc := bootstrap.ClientConfig{
					Token:         testutil.Must(tokens.FromBootstrapToken(token)),
					Endpoint:      "\x7f",
					TrustStrategy: pkpTrustStrategy(cert.Leaf),
				}
				kr, err := cc.Bootstrap(context.Background(), fooIdent)
				Expect(kr).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("net/url"))
			})
		})
		When("the client fails to send a request to the server", func() {
			It("should error", func() {
				token, _ := store.CreateToken(context.Background(), 1*time.Minute)
				cc := bootstrap.ClientConfig{
					Token:         testutil.Must(tokens.FromBootstrapToken(token)),
					Endpoint:      "localhost:65545",
					TrustStrategy: pkpTrustStrategy(cert.Leaf),
				}
				kr, err := cc.Bootstrap(context.Background(), fooIdent)
				Expect(kr).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("invalid port"))
			})
		})
	})
})
