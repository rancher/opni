package test

import (
	"context"
	"crypto/tls"
	"crypto/x509"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	MockCAName         = "mock-ca-cert"
	MockClientCertName = "mock-client-cert"
)

type TestCertManager struct{}

func (m *TestCertManager) PopulateK8sObjects(ctx context.Context, client ctrlclient.Client, namespace string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MockCAName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"ca.crt": TestData("root_ca.crt"),
			"ca.key": TestData("root_ca.key"),
		},
	}
	err := client.Create(ctx, secret)
	if err != nil {
		return err
	}
	clientsecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MockCAName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"tls.crt": TestData("localhost.crt"),
			"tls.key": TestData("localhost.key"),
		},
	}
	return client.Create(ctx, clientsecret)
}

func (m *TestCertManager) GenerateRootCACert() error {
	return nil
}

func (m *TestCertManager) GenerateTransportCA() error {
	return nil
}

func (m *TestCertManager) GenerateHTTPCA() error {
	return nil
}

func (m *TestCertManager) GenerateClientCert(user string) error {
	return nil
}

func (m *TestCertManager) GenerateAdminClientCert() error {
	return nil
}

func (m *TestCertManager) GetTransportRootCAs() (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(TestData("root_ca.crt"))
	return pool, nil
}

func (m *TestCertManager) GetHTTPRootCAs() (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(TestData("root_ca.crt"))
	return pool, nil
}

func (m *TestCertManager) GetClientCert(user string) (tls.Certificate, error) {
	return tls.Certificate{}, nil
}

func (m *TestCertManager) GetAdminClientCert() (tls.Certificate, error) {
	return tls.Certificate{}, nil
}

func (m *TestCertManager) GetTransportCARef() (corev1.LocalObjectReference, error) {
	return corev1.LocalObjectReference{
		Name: MockCAName,
	}, nil
}

func (m *TestCertManager) GetHTTPCARef() (corev1.LocalObjectReference, error) {
	return corev1.LocalObjectReference{
		Name: MockCAName,
	}, nil
}

func (m *TestCertManager) GetClientCertRef(_ string) (corev1.LocalObjectReference, error) {
	return corev1.LocalObjectReference{
		Name: MockClientCertName,
	}, nil
}
