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
	"strings"

	"github.com/kralicky/opni-gateway/pkg/ecdh"
	"github.com/kralicky/opni-gateway/pkg/keyring"
	"golang.org/x/crypto/blake2b"
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
	Token      *Token
	CACertHash []byte
	Endpoint   string
	Ident      IdentProvider
}

func (c *ClientConfig) Bootstrap() (keyring.Keyring, error) {
	response, serverLeafCert, err := c.bootstrapInsecure()
	if err != nil {
		return nil, err
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
	secureReq, err := json.Marshal(SecureBootstrapRequest{
		ClientID:     c.Ident.UniqueIdentifier(context.Background()),
		ClientPubKey: ekp.PublicKey,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, c.Endpoint,
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

func (c *ClientConfig) bootstrapInsecure() (*BootstrapResponse, *x509.Certificate, error) {
	// Ensure the endpoint is https
	url := c.Endpoint
	if strings.HasPrefix(url, "http://") {
		return nil, nil, fmt.Errorf("%w: use https:// instead of http:// or omit the protocol", ErrInvalidEndpoint)
	}
	if !strings.HasPrefix(url, "https://") {
		url = fmt.Sprintf("https://%s", url)
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
	resp, err := insecureClient.Post(url, "application/json", nil)
	if err != nil || resp.StatusCode != 200 {
		return nil, nil, err
	}
	defer resp.Body.Close()
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
	responseCA, err := x509.ParseCertificate(resp.CACert)
	if err != nil {
		return err
	}
	certs := tls.PeerCertificates
	peerRootCA := certs[len(certs)-1]
	if !peerRootCA.IsCA || !responseCA.IsCA {
		return ErrNoRootCA
	}

	hashA := blake2b.Sum256(peerRootCA.RawSubjectPublicKeyInfo)
	hashB := blake2b.Sum256(responseCA.RawSubjectPublicKeyInfo)
	if subtle.ConstantTimeCompare(hashA[:], hashB[:]) != 1 {
		return ErrRootCAHashMismatch
	}
	if subtle.ConstantTimeCompare(c.CACertHash, hashA[:]) != 1 {
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
