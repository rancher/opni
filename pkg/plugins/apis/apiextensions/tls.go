package apiextensions

import (
	"crypto/tls"

	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/util"
)

func (tc *CertConfig) TLSConfig() (*tls.Config, error) {
	certCfg := v1beta1.CertsSpec{}
	if tc.Ca != "" {
		certCfg.CACert = &tc.Ca
	}
	if tc.CaData != "" {
		certCfg.CACertData = &tc.CaData
	}
	if tc.Cert != "" {
		certCfg.ServingCert = &tc.Cert
	}
	if tc.CertData != "" {
		certCfg.ServingCertData = &tc.CertData
	}
	if tc.Key != "" {
		certCfg.ServingKey = &tc.Key
	}
	if tc.KeyData != "" {
		certCfg.ServingKeyData = &tc.KeyData
	}
	bundle, caPool, err := util.LoadServingCertBundle(certCfg)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      caPool,
		Certificates: []tls.Certificate{*bundle},
	}, nil
}

func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func NewCertConfig(certs v1beta1.CertsSpec) *CertConfig {
	return &CertConfig{
		Ca:       strOrEmpty(certs.CACert),
		CaData:   strOrEmpty(certs.CACertData),
		Cert:     strOrEmpty(certs.ServingCert),
		CertData: strOrEmpty(certs.ServingCertData),
		Key:      strOrEmpty(certs.ServingKey),
		KeyData:  strOrEmpty(certs.ServingKeyData),
	}
}
