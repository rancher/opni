package tokens_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/hex"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/tokens"
)

func mustParse(t *tokens.Token, err error) *tokens.Token {
	if err != nil {
		panic(err)
	}
	return t
}

var (
	sampleHexToken  = "abcdef012345.0123456789abcdef0123456789abcdef0123456789abcdef0123"
	sampleJsonToken = `{"id":"q83vASNF","secret":"ASNFZ4mrze8BI0VniavN7wEjRWeJq83vASM="}`
	entries         = []TableEntry{
		Entry("Using crypto/rand.Reader explicitly", tokens.NewToken(rand.Reader)),
		Entry("Using default reader", tokens.NewToken()),
		Entry("Parsing a hex token", mustParse(tokens.ParseHex(sampleHexToken))),
		Entry("Parsing a json token", mustParse(tokens.ParseJSON([]byte(sampleJsonToken)))),
	}
)

var _ = Describe("Token", Label("unit"), func() {
	DescribeTable("Token sections should contain the expected data", func(token *tokens.Token) {
		Expect(len(token.ID)).To(Equal(6))
		Expect(len(token.Secret)).To(Equal(26))
		Expect(hex.DecodeString(token.HexID())).To(Equal(token.ID))
	}, entries)
	DescribeTable("Tokens should convert between various formats", func(token *tokens.Token) {
		Expect(tokens.ParseHex(token.EncodeHex())).To(Equal(token))
		Expect(tokens.ParseJSON(token.EncodeJSON())).To(Equal(token))
		Expect(tokens.ParseJSON(mustParse(tokens.ParseHex(token.EncodeHex())).EncodeJSON())).To(Equal(token))
		Expect(tokens.ParseHex(mustParse(tokens.ParseJSON(token.EncodeJSON())).EncodeHex())).To(Equal(token))
	}, entries)
	It("should handle errors", func() {
		t, err := tokens.ParseHex("")
		Expect(t).To(BeNil())
		Expect(err).To(HaveOccurred())

		t, err = tokens.ParseHex(sampleJsonToken)
		Expect(t).To(BeNil())
		Expect(err).To(HaveOccurred())

		t, err = tokens.ParseJSON([]byte(sampleHexToken))
		Expect(t).To(BeNil())
		Expect(err).To(HaveOccurred())

		Expect(func() {
			tokens.NewToken(io.LimitReader(rand.Reader, 50))
		}).To(Panic())

		t, err = tokens.ParseHex("abcdef012345.0123456789abcdef0123456789abcdef0123456789zzzzzz0123")
		Expect(t).To(BeNil())
		Expect(err).To(HaveOccurred())

		t, err = tokens.ParseHex("zzzzzz012345.0123456789abcdef0123456789abcdef0123456789zzzzzz0123")
		Expect(t).To(BeNil())
		Expect(err).To(HaveOccurred())
	})

	It("should sign and verify tokens", func() {
		t := tokens.NewToken()
		pub, priv, err := ed25519.GenerateKey(nil)
		Expect(err).NotTo(HaveOccurred())
		sig, err := t.SignDetached(priv)
		Expect(err).NotTo(HaveOccurred())
		segments := bytes.Split(sig, []byte{'.'})
		Expect(len(segments)).To(Equal(3))
		Expect(segments[1]).To(BeEmpty())
		header, err := base64.RawURLEncoding.DecodeString(string(segments[0]))
		Expect(err).NotTo(HaveOccurred())
		Expect(header).To(BeEquivalentTo(`{"alg":"EdDSA"}`))
		complete, err := t.VerifyDetached(sig, pub)
		Expect(err).NotTo(HaveOccurred())
		segments = bytes.Split(complete, []byte{'.'})
		Expect(len(segments)).To(Equal(3))
		Expect(base64.RawURLEncoding.DecodeString(string(segments[1]))).To(BeEquivalentTo(t.EncodeJSON()))
	})
	It("should handle errors when signing", func() {
		t := tokens.NewToken()
		wrongKeyType, err := rsa.GenerateKey(rand.Reader, 2048)
		Expect(err).NotTo(HaveOccurred())
		_, err = t.SignDetached(wrongKeyType)
		Expect(err).To(HaveOccurred())
		_, err = t.SignDetached(nil)
		Expect(err).To(HaveOccurred())
	})
	It("should handle errors when verifying", func() {
		t := tokens.NewToken()
		pubA, privA, err := ed25519.GenerateKey(nil)
		Expect(err).NotTo(HaveOccurred())
		pubB, privB, err := ed25519.GenerateKey(nil)
		Expect(err).NotTo(HaveOccurred())

		sig, err := t.SignDetached(privA)
		Expect(err).NotTo(HaveOccurred())

		_, err = t.VerifyDetached(sig, pubB)
		Expect(err).To(HaveOccurred())

		_, err = t.VerifyDetached(sig, pubA)
		Expect(err).NotTo(HaveOccurred())

		_, err = t.VerifyDetached([]byte("xyz"), pubA)
		Expect(err).To(HaveOccurred())

		_, err = t.VerifyDetached([]byte("xyz."), pubA)
		Expect(err).To(HaveOccurred())

		_, err = t.VerifyDetached([]byte(".xyz."), pubA)
		Expect(err).To(HaveOccurred())

		_, err = t.VerifyDetached([]byte(".xyz"), pubA)
		Expect(err).To(HaveOccurred())

		_, err = t.VerifyDetached([]byte(""), pubA)
		Expect(err).To(HaveOccurred())

		_, err = t.VerifyDetached(sig, privB)
		Expect(err).To(HaveOccurred())

		_, err = t.VerifyDetached(sig, privA)
		Expect(err).NotTo(HaveOccurred())

		sig, err = t.SignDetached(privB)
		Expect(err).NotTo(HaveOccurred())

		_, err = t.VerifyDetached(sig, pubA)
		Expect(err).To(HaveOccurred())

		_, err = t.VerifyDetached(sig, pubB)
		Expect(err).NotTo(HaveOccurred())

		_, err = t.VerifyDetached(sig, privA)
		Expect(err).To(HaveOccurred())

		_, err = t.VerifyDetached(sig, privB)
		Expect(err).NotTo(HaveOccurred())
	})
	It("should correctly generate a reference", func() {
		t := tokens.NewToken()
		Expect(t.Reference()).To(Equal(&corev1.Reference{
			Id: t.HexID(),
		}))
	})
})
