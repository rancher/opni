package apiextensions

import (
	"crypto/tls"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
)

func (tc *CertConfig) TLSConfig() (*tls.Config, error) {
	certCfg := v1beta1.CertsSpec{}
	if tc.Ca != "" {
		certCfg.CACert = &tc.Ca
	}
	if tc.CaData != nil {
		certCfg.CACertData = tc.CaData
	}
	if tc.Cert != "" {
		certCfg.ServingCert = &tc.Cert
	}
	if tc.CertData != nil {
		certCfg.ServingCertData = tc.CertData
	}
	if tc.Key != "" {
		certCfg.ServingKey = &tc.Key
	}
	if tc.KeyData != nil {
		certCfg.ServingKeyData = tc.KeyData
	}
	bundle, caPool, err := util.LoadServingCertBundle(certCfg)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
		Certificates: []tls.Certificate{*bundle},
	}, nil
}

func NewCertConfig(certs v1beta1.CertsSpec) *CertConfig {
	return &CertConfig{
		Ca:       lo.FromPtr(certs.CACert),
		CaData:   certs.CACertData,
		Cert:     lo.FromPtr(certs.ServingCert),
		CertData: certs.ServingCertData,
		Key:      lo.FromPtr(certs.ServingKey),
		KeyData:  certs.ServingKeyData,
	}
}

func NewInsecureCertConfig() *CertConfig {
	return &CertConfig{
		Insecure: true,
	}
}
