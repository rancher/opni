package bootstrap

import (
	"bytes"
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/kralicky/opni-gateway/pkg/ecdh"
	"github.com/kralicky/opni-gateway/pkg/ident"
	"github.com/kralicky/opni-gateway/pkg/keyring"
	"github.com/kralicky/opni-gateway/pkg/tokens"
	"github.com/kralicky/opni-gateway/pkg/util"
)

var (
	ErrInvalidEndpoint    = errors.New("invalid endpoint")
	ErrNoRootCA           = errors.New("no root CA found in peer certificates")
	ErrLeafNotSigned      = errors.New("leaf certificate not signed by the root CA")
	ErrKeyExpired         = errors.New("key expired")
	ErrRootCAHashMismatch = errors.New("root CA hash mismatch")
	ErrBootstrapFailed    = errors.New("bootstrap failed")
	ErrNoValidSignature   = errors.New("no valid signature found in response")
)

type ClientConfig struct {
	Token      *tokens.Token
	CACertHash []byte
	Endpoint   string
	Ident      ident.Provider
}

func (c *ClientConfig) Bootstrap(ctx context.Context) (keyring.Keyring, error) {
	response, serverLeafCert, err := c.bootstrapInsecure()
	if err != nil {
		return nil, err
	}

	if len(response.Signatures) == 0 {
		return nil, errors.New("server has no active bootstrap tokens")
	}

	completeJws, err := c.findValidSignature(
		response.Signatures, serverLeafCert.PublicKey)
	if err != nil {
		return nil, err
	}

	cacert, err := x509.ParseCertificate(response.CACert)
	if err != nil {
		return nil, err
	}

	rootCAPool := x509.NewCertPool()
	rootCAPool.AddCert(cacert)
	tlsConfig := &tls.Config{
		RootCAs:          rootCAPool,
		CurvePreferences: []tls.CurveID{tls.X25519},
		ServerName:       serverLeafCert.Subject.CommonName,
	}
	secureClient := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	ekp, err := ecdh.NewEphemeralKeyPair()
	if err != nil {
		return nil, err
	}
	secureReq, err := json.Marshal(SecureBootstrapRequest{
		ClientID:     c.Ident.UniqueIdentifier(context.Background()),
		ClientPubKey: ekp.PublicKey,
	})
	if err != nil {
		return nil, err
	}

	url, _ := c.bootstrapURL()
	req, err := http.NewRequest(http.MethodPost, url.String(),
		bytes.NewReader(secureReq))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Request", "application/json")
	req.Header.Add("Authorization", "Bearer "+string(completeJws))
	resp, err := secureClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return nil, fmt.Errorf("%w: %s", ErrBootstrapFailed, resp.Status)
	}
	defer resp.Body.Close()

	var secureResp SecureBootstrapResponse
	if err := json.NewDecoder(resp.Body).Decode(&secureResp); err != nil {
		return nil, err
	}

	sharedSecret, err := ecdh.DeriveSharedSecret(ekp, ecdh.PeerPublicKey{
		PublicKey: secureResp.ServerPubKey,
		PeerType:  ecdh.PeerTypeServer,
	})
	return keyring.New(
		keyring.NewClientKeys(sharedSecret),
		keyring.NewTLSKeys(tlsConfig),
	), nil
}

func (c *ClientConfig) bootstrapURL() (*url.URL, error) {
	u, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" {
		u.Scheme = "https"
	}
	if u.Path != "bootstrap" {
		u.Path = "bootstrap"
	}
	return u, nil
}

func (c *ClientConfig) bootstrapInsecure() (*BootstrapResponse, *x509.Certificate, error) {
	url, err := c.bootstrapURL()
	if err != nil {
		return nil, nil, err
	}
	// Connect to the endpoint insecurely, the server will return a jws with a
	// detached payload that we can reconstruct and verify
	insecureClient := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	resp, err := insecureClient.Post(url.String(), "application/json", nil)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, nil, fmt.Errorf("%w: %s", ErrBootstrapFailed, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	bootstrapResponse := &BootstrapResponse{}
	if err := json.Unmarshal(body, bootstrapResponse); err != nil {
		return nil, nil, err
	}
	if err := c.validateServerCA(bootstrapResponse, resp.TLS); err != nil {
		return nil, nil, err
	}
	return bootstrapResponse, resp.TLS.PeerCertificates[0], nil
}

func (c *ClientConfig) validateServerCA(
	resp *BootstrapResponse,
	tls *tls.ConnectionState,
) error {
	cacert, err := x509.ParseCertificate(resp.CACert)
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	roots.AddCert(cacert)
	leaf := tls.PeerCertificates[0]
	intermediates := x509.NewCertPool()
	for _, cert := range tls.PeerCertificates[1:] {
		intermediates.AddCert(cert)
	}
	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
	}
	if _, err := leaf.Verify(opts); err != nil {
		return err
	}
	actual, err := util.CACertHash(cacert)
	if err != nil {
		return err
	}
	expected := c.CACertHash
	if subtle.ConstantTimeCompare(actual, expected) != 1 {
		return ErrRootCAHashMismatch
	}
	return nil
}

func (c *ClientConfig) findValidSignature(
	signatures map[string][]byte,
	pubKey interface{},
) ([]byte, error) {
	if sig, ok := signatures[c.Token.HexID()]; ok {
		return c.Token.VerifyDetached(sig, pubKey)
	}
	return nil, ErrNoValidSignature
}
