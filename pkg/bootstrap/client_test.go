package bootstrap_test

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jws"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kralicky/opni-monitoring/pkg/bootstrap"
	"github.com/kralicky/opni-monitoring/pkg/ecdh"
	"github.com/kralicky/opni-monitoring/pkg/ident"
	"github.com/kralicky/opni-monitoring/pkg/pkp"
	"github.com/kralicky/opni-monitoring/pkg/test"
	"github.com/kralicky/opni-monitoring/pkg/tokens"
)

var _ = Describe("Client", func() {
	It("should bootstrap with the server", func() {
		token := tokens.NewToken()
		mux := http.NewServeMux()

		cert, err := tls.X509KeyPair(test.TestData("self_signed_leaf.crt"), test.TestData("self_signed_leaf.key"))
		Expect(err).NotTo(HaveOccurred())

		mux.HandleFunc("/bootstrap/join", func(rw http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			Expect(r.Header.Get("Authorization")).To(BeEmpty())
			data, err := token.SignDetached(cert.PrivateKey)
			Expect(err).NotTo(HaveOccurred())
			rw.WriteHeader(http.StatusOK)
			j, _ := json.Marshal(bootstrap.BootstrapJoinResponse{
				Signatures: map[string][]byte{
					token.HexID(): data,
				},
			})
			_, err = rw.Write(j)
			Expect(err).NotTo(HaveOccurred())
		})
		mux.HandleFunc("/bootstrap/auth", func(rw http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			Expect(r.Header.Get("Authorization")).NotTo(BeEmpty())
			authHeader := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")

			_, err := jws.Verify([]byte(authHeader), jwa.EdDSA, cert.PrivateKey.(ed25519.PrivateKey).Public())
			Expect(err).NotTo(HaveOccurred())

			req := bootstrap.BootstrapAuthRequest{}
			Expect(json.NewDecoder(r.Body).Decode(&req)).To(Succeed())
			Expect(req.ClientID).To(Equal("foo"))
			Expect(req.ClientPubKey).To(HaveLen(32))

			ekp, err := ecdh.NewEphemeralKeyPair()
			Expect(err).NotTo(HaveOccurred())
			rw.WriteHeader(http.StatusOK)
			resp, _ := json.Marshal(bootstrap.BootstrapAuthResponse{
				ServerPubKey: ekp.PublicKey,
			})
			_, err = rw.Write(resp)
			Expect(err).NotTo(HaveOccurred())
		})
		server := httptest.NewUnstartedServer(mux)
		server.TLS = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
		server.StartTLS()
		defer server.Close()

		cc := bootstrap.ClientConfig{
			Token:    token,
			Pins:     []*pkp.PublicKeyPin{pkp.NewSha256(server.Certificate())},
			Endpoint: server.URL,
		}

		_, err = cc.Bootstrap(context.Background(), ident.NewFakeProvider("foo"))
		Expect(err).NotTo(HaveOccurred())
	})
})
