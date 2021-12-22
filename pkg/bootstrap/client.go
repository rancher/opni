package bootstrap

import (
	"bytes"
	"context"
	"crypto"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kralicky/opni-gateway/pkg/ecdh"
	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jws"
	"github.com/lestrrat-go/jwx/jwt"
)

var (
	ErrInvalidEndpoint    = errors.New("invalid endpoint")
	ErrNoRootCA           = errors.New("no root CA found in peer certificates")
	ErrLeafNotSigned      = errors.New("leaf certificate not signed by the root CA")
	ErrRootCAHashMismatch = errors.New("root CA hash mismatch")
	ErrBootstrapFailed    = errors.New("bootstrap failed")
)

type ClientConfig struct {
	Token      string
	CACertHash string
	Endpoint   string
	Ident      IdentProvider
}

func (b ClientConfig) Run() (jwt.Token, crypto.PrivateKey, error) {
	// Ensure the endpoint is https
	url := b.Endpoint
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
	resp, err := insecureClient.Post(url, "application/x-www-form-urlencoded", nil)
	if err != nil {
		return nil, nil, err
	}
	// Check the peer certificates - there should be a leaf certificate used
	// to sign the JWS and for tls, and a root CA cert in the chain
	certs := resp.TLS.PeerCertificates
	signingCert := certs[0]
	rootCA := certs[len(certs)-1]

	if !rootCA.IsCA {
		return nil, nil, ErrNoRootCA
	}

	// Check that the leaf was signed by the root
	if err := rootCA.CheckSignature(
		signingCert.SignatureAlgorithm,
		signingCert.RawTBSCertificate,
		signingCert.Signature,
	); err != nil {
		return nil, nil, ErrLeafNotSigned
	}

	// Check root ca hash (only subject public key info)
	hash := sha256.Sum256(rootCA.RawSubjectPublicKeyInfo)
	if hex.EncodeToString(hash[:]) != b.CACertHash {
		return nil, nil, ErrRootCAHashMismatch
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	// Reconstruct the JWS with the bootstrap token and verify
	payload := base64.RawURLEncoding.EncodeToString([]byte(b.Token))
	_, err = jws.Verify(body, jwa.EdDSA, signingCert.PublicKey,
		jws.WithDetachedPayload([]byte(payload)))
	if err != nil {
		return nil, nil, err
	}

	// At this point we trust the server and can send them back the jwt
	// containing the bootstrap token, and an ephemeral public key

	completeJws := bytes.Replace(body, []byte(".."), []byte("."+payload+"."), 1)
	rootCAPool := x509.NewCertPool()
	rootCAPool.AddCert(rootCA)
	secureClient := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:          rootCAPool,
				CurvePreferences: []tls.CurveID{tls.X25519},
				ServerName:       signingCert.Subject.CommonName,
			},
		},
	}

	ekp, err := ecdh.NewEphemeralKeyPair()
	secureReq, err := json.Marshal(SecureBootstrapRequest{
		ClientID:     b.Ident.UniqueIdentifier(context.Background()),
		ClientPubKey: ekp.PublicKey,
	})
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(secureReq))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Request", "application/json")
	req.Header.Add("Authorization", "Bearer "+string(completeJws))
	resp, err = secureClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != 200 {
		return nil, nil, fmt.Errorf("%w: %s", ErrBootstrapFailed, resp.Status)
	}
	defer resp.Body.Close()
	// The server sends us back a unique token we can use to authenticate all
	// future requests - this token does not expire (todo)
	// The token's sub header is our unqiue client ID
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read server response: %w", err)
	}
	token, err := jwt.Parse(body, jwt.WithVerify(jwa.EdDSA, signingCert.PublicKey))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse server response: %w", err)
	}
	return token, ekp, nil
}
