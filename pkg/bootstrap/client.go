package bootstrap

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/kralicky/opni-monitoring/pkg/ecdh"
	"github.com/kralicky/opni-monitoring/pkg/ident"
	"github.com/kralicky/opni-monitoring/pkg/keyring"
	"github.com/kralicky/opni-monitoring/pkg/pkp"
	"github.com/kralicky/opni-monitoring/pkg/tokens"
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
	Token    *tokens.Token
	Pins     []*pkp.PublicKeyPin
	Endpoint string
}

func (c *ClientConfig) Bootstrap(
	ctx context.Context,
	ident ident.Provider,
) (keyring.Keyring, error) {
	response, serverLeafCert, err := c.bootstrapJoin()
	if err != nil {
		return nil, err
	}

	completeJws, err := c.findValidSignature(
		response.Signatures, serverLeafCert.PublicKey)
	if err != nil {
		return nil, err
	}

	// error already checked in bootstrapJoin
	tlsConfig, _ := pkp.TLSConfig(c.Pins)

	client := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	ekp := ecdh.NewEphemeralKeyPair()
	id, err := ident.UniqueIdentifier(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to obtain unique identifier: %w", err)
	}
	authReq, err := json.Marshal(BootstrapAuthRequest{
		ClientID:     id,
		ClientPubKey: ekp.PublicKey,
	})
	if err != nil {
		return nil, err
	}

	// error already checked in bootstrapJoin
	url, _ := c.bootstrapAuthURL()

	req, err := http.NewRequest(http.MethodPost, url.String(),
		bytes.NewReader(authReq))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Request", "application/json")
	req.Header.Add("Authorization", "Bearer "+string(completeJws))
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%w: %s", ErrBootstrapFailed, resp.Status)
	}
	defer resp.Body.Close()

	var authResp BootstrapAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return nil, err
	}

	sharedSecret, err := ecdh.DeriveSharedSecret(ekp, ecdh.PeerPublicKey{
		PublicKey: authResp.ServerPubKey,
		PeerType:  ecdh.PeerTypeServer,
	})
	if err != nil {
		return nil, err
	}
	return keyring.New(
		keyring.NewSharedKeys(sharedSecret),
		keyring.NewPKPKey(c.Pins),
	), nil
}

func (c *ClientConfig) bootstrapJoinURL() (*url.URL, error) {
	u, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, err
	}
	u.Scheme = "https"
	u.Path = "bootstrap/join"
	return u, nil
}

func (c *ClientConfig) bootstrapAuthURL() (*url.URL, error) {
	u, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, err
	}
	u.Scheme = "https"
	u.Path = "bootstrap/auth"
	return u, nil
}

func (c *ClientConfig) bootstrapJoin() (*BootstrapJoinResponse, *x509.Certificate, error) {
	url, err := c.bootstrapJoinURL()
	if err != nil {
		return nil, nil, err
	}

	tlsConfig, err := pkp.TLSConfig(c.Pins)
	if err != nil {
		return nil, nil, err
	}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}
	resp, err := client.Post(url.String(), "application/json", nil)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, nil, fmt.Errorf(resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	bootstrapResponse := &BootstrapJoinResponse{}
	if err := json.Unmarshal(body, bootstrapResponse); err != nil {
		return nil, nil, err
	}

	return bootstrapResponse, resp.TLS.PeerCertificates[0], nil
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
