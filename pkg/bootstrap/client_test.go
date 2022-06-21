package bootstrap_test

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"os"
	"runtime"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"

	bootstrapv1 "github.com/rancher/opni/pkg/apis/bootstrap/v1"
	"github.com/rancher/opni/pkg/bootstrap"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/config/meta"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/pkp"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test"
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
		fooIdent = test.NewTestIdentProvider(ctrl, "foo")
		var err error
		crt, err := tls.X509KeyPair(test.TestData("self_signed_leaf.crt"), test.TestData("self_signed_leaf.key"))
		Expect(err).NotTo(HaveOccurred())
		crt.Leaf, err = x509.ParseCertificate(crt.Certificate[0])
		Expect(err).NotTo(HaveOccurred())
		cert = &crt
		store = test.NewTestStorageBackend(context.Background(), ctrl)
		capBackendStore := capabilities.NewBackendStore(capabilities.ServerInstallerTemplateSpec{}, test.Log)
		for _, backend := range []*test.CapabilityInfo{
			{
				Name:       "test",
				CanInstall: true,
			},
		} {
			capBackendStore.Add(backend.Name, test.NewTestCapabilityBackend(ctrl, backend))
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
	When("the bootstrap process is complete", func() {
		It("should erase bootstrap tokens from the config secret", func() {
			if runtime.GOOS != "linux" {
				Skip("skipping test on non-linux OS")
			}
			env := test.Environment{
				TestBin: "../../testbin/bin",
			}
			k8sConfig, err := env.StartK8s()
			Expect(err).NotTo(HaveOccurred())

			os.Setenv("POD_NAMESPACE", "default")
			client, err := kubernetes.NewForConfig(k8sConfig)
			Expect(err).NotTo(HaveOccurred())

			config := v1beta1.AgentConfig{
				TypeMeta: meta.TypeMeta{
					APIVersion: "v1beta1",
					Kind:       "AgentConfig",
				},
				Spec: v1beta1.AgentConfigSpec{
					TrustStrategy:    v1beta1.TrustStrategyPKP,
					IdentityProvider: "foo",
					Bootstrap: &v1beta1.BootstrapSpec{
						Token: "foo",
						Pins:  []string{"foo", "bar"},
					},
				},
			}
			data, err := yaml.Marshal(config)
			Expect(err).NotTo(HaveOccurred())
			_, err = client.CoreV1().Secrets("default").
				Create(context.Background(), &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "agent-config",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"config.yaml": data,
					},
				}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			cc := bootstrap.ClientConfig{
				K8sConfig:    k8sConfig,
				K8sNamespace: "default",
			}
			err = cc.Finalize(context.Background())
			Expect(err).NotTo(HaveOccurred())

			secret, err := client.CoreV1().Secrets("default").
				Get(context.Background(), "agent-config", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			erased := v1beta1.AgentConfig{}
			Expect(yaml.Unmarshal(secret.Data["config.yaml"], &erased)).To(Succeed())
			Expect(erased.Spec.IdentityProvider).To(Equal("foo"))
			Expect(erased.Spec.Bootstrap).To(BeNil())

			Expect(env.Stop()).To(Succeed())
		})
		When("the defaults are used and the in-cluster config is unavailable", func() {
			It("should error", func() {
				// sanity check, this is set above
				Expect(os.Getenv("POD_NAMESPACE")).To(Equal("default"))

				cc := bootstrap.ClientConfig{
					K8sConfig: nil,
				}
				err := cc.Finalize(context.Background())
				Expect(err).To(MatchError(rest.ErrNotInCluster))
			})
		})
		When("a namespace is not found or configured", func() {
			It("should error", func() {
				// This is set above
				os.Unsetenv("POD_NAMESPACE")

				cc := bootstrap.ClientConfig{
					K8sConfig:    nil,
					K8sNamespace: "",
				}
				err := cc.Finalize(context.Background())
				Expect(err).To(MatchError("POD_NAMESPACE not set, and no namespace was explicitly configured"))
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
