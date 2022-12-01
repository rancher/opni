package certs

import (
	"context"
	"crypto/tls"
	"crypto/x509"

	corev1 "k8s.io/api/core/v1"
)

type OpensearchCertManager interface {
	OpensearchCertWriter
	OpensearchCertReader
}

type OpensearchCertWriter interface {
	GenerateRootCACert() error
	GenerateTransportCA() error
	GenerateHTTPCA() error
	GenerateClientCert(user string) error
}

type OpensearchCertReader interface {
	GetTransportRootCAs() (*x509.CertPool, error)
	GetHTTPRootCAs() (*x509.CertPool, error)
	GetClientCertificate(user string) (tls.Certificate, error)
}

type K8sOpensearchCertManager interface {
	GetTransportCARef(context.Context) (corev1.LocalObjectReference, error)
	GetHTTPCARef(context.Context) (corev1.LocalObjectReference, error)
}
