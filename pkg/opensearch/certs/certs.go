package certs

import (
	"crypto/tls"
	"crypto/x509"

	corev1 "k8s.io/api/core/v1"
)

type OpensearchCertManager interface {
	OpensearchCertWriter
	OpensearchCertReader
}

type OpensearchCertReconcile interface {
	OpensearchCertManager
	K8sOpensearchCertManager
}

type OpensearchCertWriter interface {
	GenerateRootCACert() error
	GenerateTransportCA() error
	GenerateHTTPCA() error
	GenerateAdminClientCert() error
	GenerateClientCert(user string) error
}

type OpensearchCertReader interface {
	GetTransportRootCAs() (*x509.CertPool, error)
	GetHTTPRootCAs() (*x509.CertPool, error)
	GetClientCert(user string) (tls.Certificate, error)
	GetAdminClientCert() (tls.Certificate, error)
}

type K8sOpensearchCertManager interface {
	GetTransportCARef() (corev1.LocalObjectReference, error)
	GetHTTPCARef() (corev1.LocalObjectReference, error)
	GetClientCertRef(user string) (corev1.LocalObjectReference, error)
}
