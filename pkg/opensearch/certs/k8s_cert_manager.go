package certs

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"time"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	backoffv2 "github.com/lestrrat-go/backoff/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	scheme *runtime.Scheme
)

type certMgrOpensearchManager struct {
	certMgrOptions
	ctx       context.Context
	k8sClient ctrlclient.Client
}

type certMgrOptions struct {
	storageNamespace string
	cluster          string
	restconfig       *rest.Config
}

type CertMgrOption func(*certMgrOptions)

func (o *certMgrOptions) apply(opts ...CertMgrOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithNamespace(namespace string) CertMgrOption {
	return func(o *certMgrOptions) {
		o.storageNamespace = namespace
	}
}

func WithCluster(cluster string) CertMgrOption {
	return func(o *certMgrOptions) {
		o.cluster = cluster
	}
}

func WithRestConfig(restconfig *rest.Config) CertMgrOption {
	return func(o *certMgrOptions) {
		o.restconfig = restconfig
	}
}

func NewCertMgrOpensearchCertManager(ctx context.Context, opts ...CertMgrOption) OpensearchCertManager {
	options := certMgrOptions{
		cluster: "opni",
	}
	options.apply(opts...)

	var restconfig *rest.Config
	if options.restconfig != nil {
		restconfig = options.restconfig
	} else {
		rest, err := rest.InClusterConfig()
		if err != nil {
			panic(err)
		}
		restconfig = rest
	}

	client, err := ctrlclient.New(restconfig, ctrlclient.Options{
		Scheme: scheme,
	})
	if err != nil {
		panic(err)
	}

	return &certMgrOpensearchManager{
		certMgrOptions: options,
		ctx:            ctx,
		k8sClient:      client,
	}
}

func (m *certMgrOpensearchManager) GenerateRootCACert() error {
	issuer := m.generateSelfSignedIssuer()
	err := m.k8sClient.Create(m.ctx, issuer)
	if ctrlclient.IgnoreAlreadyExists(err) != nil {
		return nil
	}

	rootCA := m.generateRootCA()
	err = m.k8sClient.Create(m.ctx, rootCA)
	if ctrlclient.IgnoreAlreadyExists(err) != nil {
		return nil
	}

	rootIssuer := m.generateRootIssuer()
	err = m.k8sClient.Create(m.ctx, rootIssuer)
	return ctrlclient.IgnoreAlreadyExists(err)
}

func (m *certMgrOpensearchManager) GenerateTransportCA() error {
	ca := m.generateTransportIntermediateCA()
	err := m.k8sClient.Create(m.ctx, ca)
	if ctrlclient.IgnoreAlreadyExists(err) != nil {
		return nil
	}

	issuer := m.generateTransportIntermediateIssuer()
	err = m.k8sClient.Create(m.ctx, issuer)
	return ctrlclient.IgnoreAlreadyExists(err)
}

func (m *certMgrOpensearchManager) GenerateHTTPCA() error {
	ca := m.generateHTTPIntermediateCA()
	err := m.k8sClient.Create(m.ctx, ca)
	if ctrlclient.IgnoreAlreadyExists(err) != nil {
		return nil
	}

	issuer := m.generateHTTPIntermediateIssuer()
	err = m.k8sClient.Create(m.ctx, issuer)
	return ctrlclient.IgnoreAlreadyExists(err)
}

func (m *certMgrOpensearchManager) GenerateClientCert(user string) error {
	cert := m.generateClientCert(user)
	err := m.k8sClient.Create(m.ctx, cert)
	return ctrlclient.IgnoreAlreadyExists(err)
}

func (m *certMgrOpensearchManager) GenerateAdminClientCert() error {
	cert := m.generateAdminCert()
	err := m.k8sClient.Create(m.ctx, cert)
	return ctrlclient.IgnoreAlreadyExists(err)
}

func (m *certMgrOpensearchManager) GetTransportRootCAs() (*x509.CertPool, error) {
	pool := x509.NewCertPool()

	transport := &corev1.Secret{}
	err := m.k8sClient.Get(m.ctx, types.NamespacedName{
		Name:      m.transportSecretName(),
		Namespace: m.storageNamespace,
	}, transport)
	if err != nil {
		return nil, err
	}
	ok := pool.AppendCertsFromPEM(transport.Data["ca.crt"])
	if !ok {
		return nil, errors.New("failed to append ca crt to pool")
	}
	ok = pool.AppendCertsFromPEM(transport.Data[corev1.TLSCertKey])
	if !ok {
		return nil, errors.New("failed to append transport crt to pool")
	}
	return pool, nil
}

func (m *certMgrOpensearchManager) GetHTTPRootCAs() (*x509.CertPool, error) {
	pool := x509.NewCertPool()

	http := &corev1.Secret{}
	err := m.k8sClient.Get(m.ctx, types.NamespacedName{
		Name:      m.httpSecretName(),
		Namespace: m.storageNamespace,
	}, http)
	if err != nil {
		return nil, err
	}
	ok := pool.AppendCertsFromPEM(http.Data["ca.crt"])
	if !ok {
		return nil, errors.New("failed to append ca crt to pool")
	}
	ok = pool.AppendCertsFromPEM(http.Data[corev1.TLSCertKey])
	if !ok {
		return nil, errors.New("failed to append transport crt to pool")
	}
	return pool, nil
}

func (m *certMgrOpensearchManager) GetClientCert(user string) (tls.Certificate, error) {
	p := backoffv2.Exponential(
		backoffv2.WithMinInterval(time.Second*2),
		backoffv2.WithMaxInterval(time.Second*5),
		backoffv2.WithMaxRetries(5),
		backoffv2.WithMultiplier(1.2),
	)

	cert := &corev1.Secret{}
	var err error
	b := p.Start(m.ctx)
	for backoffv2.Continue(b) {
		err = m.k8sClient.Get(m.ctx, types.NamespacedName{
			Name:      m.clientCertName(user),
			Namespace: m.storageNamespace,
		}, cert)
		if err != nil {
			if ctrlclient.IgnoreNotFound(err) != nil {
				return tls.Certificate{}, err
			}
			continue
		}
		return tls.X509KeyPair(cert.Data[corev1.TLSCertKey], cert.Data[corev1.TLSPrivateKeyKey])
	}

	return tls.Certificate{}, err
}

func (m *certMgrOpensearchManager) GetAdminClientCert() (tls.Certificate, error) {
	p := backoffv2.Exponential(
		backoffv2.WithMinInterval(time.Second*2),
		backoffv2.WithMaxInterval(time.Second*5),
		backoffv2.WithMaxRetries(5),
		backoffv2.WithMultiplier(1.2),
	)

	cert := &corev1.Secret{}
	var err error
	b := p.Start(m.ctx)
	for backoffv2.Continue(b) {
		err = m.k8sClient.Get(m.ctx, types.NamespacedName{
			Name:      m.adminCertName(),
			Namespace: m.storageNamespace,
		}, cert)
		if err != nil {
			if ctrlclient.IgnoreNotFound(err) != nil {
				return tls.Certificate{}, err
			}
			continue
		}
		return tls.X509KeyPair(cert.Data[corev1.TLSCertKey], cert.Data[corev1.TLSPrivateKeyKey])
	}

	return tls.Certificate{}, err
}

func (m *certMgrOpensearchManager) GetTransportCARef() (corev1.LocalObjectReference, error) {
	return corev1.LocalObjectReference{
		Name: m.transportSecretName(),
	}, nil
}

func (m *certMgrOpensearchManager) GetHTTPCARef() (corev1.LocalObjectReference, error) {
	return corev1.LocalObjectReference{
		Name: m.httpSecretName(),
	}, nil
}

func (m *certMgrOpensearchManager) generateSelfSignedIssuer() *cmv1.ClusterIssuer {
	return &cmv1.ClusterIssuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.issuerName(),
			Namespace: m.storageNamespace,
		},
		Spec: cmv1.IssuerSpec{
			IssuerConfig: cmv1.IssuerConfig{
				SelfSigned: &cmv1.SelfSignedIssuer{},
			},
		},
	}
}

func (m *certMgrOpensearchManager) generateRootCA() *cmv1.Certificate {
	return &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.caSecretName(),
			Namespace: m.storageNamespace,
		},
		Spec: cmv1.CertificateSpec{
			IsCA:       true,
			CommonName: "opensearch-root-ca",
			SecretName: m.caSecretName(),
			PrivateKey: &cmv1.CertificatePrivateKey{
				Algorithm: cmv1.ECDSAKeyAlgorithm,
				Encoding:  cmv1.PKCS1,
			},
			IssuerRef: cmmetav1.ObjectReference{
				Group: "cert-manager.io",
				Kind:  "ClusterIssuer",
				Name:  m.issuerName(),
			},
		},
	}
}

func (m *certMgrOpensearchManager) generateRootIssuer() *cmv1.Issuer {
	return &cmv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.caSecretName(),
			Namespace: m.storageNamespace,
		},
		Spec: cmv1.IssuerSpec{
			IssuerConfig: cmv1.IssuerConfig{
				CA: &cmv1.CAIssuer{
					SecretName: m.caSecretName(),
				},
			},
		},
	}
}

func (m *certMgrOpensearchManager) generateTransportIntermediateCA() *cmv1.Certificate {
	return &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.transportSecretName(),
			Namespace: m.storageNamespace,
		},
		Spec: cmv1.CertificateSpec{
			IsCA:       true,
			CommonName: "opensearch-transport-intermediate",
			SecretName: m.transportSecretName(),
			PrivateKey: &cmv1.CertificatePrivateKey{
				Algorithm: cmv1.ECDSAKeyAlgorithm,
				Encoding:  cmv1.PKCS1,
			},
			DNSNames: []string{
				"opensearch-transport-intermediate",
			},
			IssuerRef: cmmetav1.ObjectReference{
				Group: "cert-manager.io",
				Kind:  "Issuer",
				Name:  m.caSecretName(),
			},
		},
	}
}

func (m *certMgrOpensearchManager) generateTransportIntermediateIssuer() *cmv1.Issuer {
	return &cmv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.transportSecretName(),
			Namespace: m.storageNamespace,
		},
		Spec: cmv1.IssuerSpec{
			IssuerConfig: cmv1.IssuerConfig{
				CA: &cmv1.CAIssuer{
					SecretName: m.transportSecretName(),
				},
			},
		},
	}
}

func (m *certMgrOpensearchManager) generateHTTPIntermediateCA() *cmv1.Certificate {
	return &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.httpSecretName(),
			Namespace: m.storageNamespace,
		},
		Spec: cmv1.CertificateSpec{
			IsCA:       true,
			CommonName: "opensearch-http-intermediate",
			SecretName: m.httpSecretName(),
			PrivateKey: &cmv1.CertificatePrivateKey{
				Algorithm: cmv1.ECDSAKeyAlgorithm,
				Encoding:  cmv1.PKCS1,
			},
			DNSNames: []string{
				"opensearch-http-intermediate",
			},
			IssuerRef: cmmetav1.ObjectReference{
				Group: "cert-manager.io",
				Kind:  "Issuer",
				Name:  m.caSecretName(),
			},
		},
	}
}

func (m *certMgrOpensearchManager) generateHTTPIntermediateIssuer() *cmv1.Issuer {
	return &cmv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.httpSecretName(),
			Namespace: m.storageNamespace,
		},
		Spec: cmv1.IssuerSpec{
			IssuerConfig: cmv1.IssuerConfig{
				CA: &cmv1.CAIssuer{
					SecretName: m.httpSecretName(),
				},
			},
		},
	}
}

func (m *certMgrOpensearchManager) generateClientCert(user string) *cmv1.Certificate {
	return &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.clientCertName(user),
			Namespace: m.storageNamespace,
		},
		Spec: cmv1.CertificateSpec{
			CommonName: user,
			SecretName: m.clientCertName(user),
			PrivateKey: &cmv1.CertificatePrivateKey{
				Algorithm: cmv1.ECDSAKeyAlgorithm,
				Encoding:  cmv1.PKCS1,
			},
			DNSNames: []string{
				user,
			},
			Usages: []cmv1.KeyUsage{
				cmv1.UsageClientAuth,
			},
			IssuerRef: cmmetav1.ObjectReference{
				Group: "cert-manager.io",
				Kind:  "Issuer",
				Name:  m.httpSecretName(),
			},
		},
	}
}

func (m *certMgrOpensearchManager) generateAdminCert() *cmv1.Certificate {
	return &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.adminCertName(),
			Namespace: m.storageNamespace,
		},
		Spec: cmv1.CertificateSpec{
			Subject: &cmv1.X509Subject{
				OrganizationalUnits: []string{
					m.cluster,
				},
			},
			CommonName: "admin",
			SecretName: m.adminCertName(),
			PrivateKey: &cmv1.CertificatePrivateKey{
				Algorithm: cmv1.ECDSAKeyAlgorithm,
				Encoding:  cmv1.PKCS1,
			},
			DNSNames: []string{
				"admin",
			},
			Usages: []cmv1.KeyUsage{
				cmv1.UsageClientAuth,
			},
			IssuerRef: cmmetav1.ObjectReference{
				Group: "cert-manager.io",
				Kind:  "Issuer",
				Name:  m.httpSecretName(),
			},
		},
	}
}

func (m *certMgrOpensearchManager) issuerName() string {
	return fmt.Sprintf("opensearch-%s-issuer", m.cluster)
}
func (m *certMgrOpensearchManager) caSecretName() string {
	return fmt.Sprintf("opensearch-%s-ca", m.cluster)
}

func (m *certMgrOpensearchManager) transportSecretName() string {
	return fmt.Sprintf("opensearch-%s-transport", m.cluster)
}

func (m *certMgrOpensearchManager) httpSecretName() string {
	return fmt.Sprintf("opensearch-%s-http", m.cluster)
}

func (m *certMgrOpensearchManager) clientCertName(user string) string {
	return fmt.Sprintf("opensearch-%s-%s", m.cluster, user)
}

func (m *certMgrOpensearchManager) adminCertName() string {
	return fmt.Sprintf("opensearch-%s-admin", m.cluster)
}

func init() {
	scheme = runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	cmv1.AddToScheme(scheme)
}
