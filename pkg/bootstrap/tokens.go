package bootstrap

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"

	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jws"
)

var ErrMalformedToken = errors.New("malformed token")

type Token struct {
	ID     []byte `json:"id"`               // bytes 0-5
	Secret []byte `json:"secret,omitempty"` // bytes 6-31
}

// Creates a new bootstrap token by reading bytes from the given random source.
// the default source is crypto/rand.Reader.
func NewToken(source ...io.Reader) *Token {
	entropy := rand.Reader
	if len(source) > 0 {
		entropy = source[0]
	}
	buf := make([]byte, 256)
	if _, err := io.ReadFull(entropy, buf); err != nil {
		panic(err)
	}
	sum := sha256.Sum256(buf)
	return &Token{
		ID:     sum[:6],
		Secret: sum[6:],
	}
}

func (t *Token) EncodeJSON() string {
	str, err := json.Marshal(t)
	if err != nil {
		panic(err)
	}
	return string(str)
}

func (t *Token) EncodeHex() string {
	return hex.EncodeToString(t.ID[:]) + "." + hex.EncodeToString(t.Secret[:])
}

func (t *Token) HexID() string {
	return hex.EncodeToString(t.ID[:])
}

func DecodeJSONToken(data []byte) (*Token, error) {
	t := &Token{}
	err := json.Unmarshal(data, t)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func DecodeHexToken(str string) (*Token, error) {
	parts := bytes.Split([]byte(str), []byte("."))
	if len(parts) != 2 ||
		len(parts[0]) != hex.EncodedLen(6) ||
		len(parts[1]) != hex.EncodedLen(26) {
		return nil, ErrMalformedToken
	}
	t := &Token{
		ID:     make([]byte, 6),
		Secret: make([]byte, 26),
	}
	if n, err := hex.Decode(t.ID[:], parts[0]); err != nil || n != 6 {
		return nil, ErrMalformedToken
	}
	if n, err := hex.Decode(t.Secret[:], parts[1]); err != nil || n != 26 {
		return nil, ErrMalformedToken
	}
	return t, nil
}

// Returns a base64-encoded JWS of the form:
// 	Header.DetachedPayload.Signature
// where DetachedPayload is the base64-encoded json string:
//  {"id":"<token_id>"}
func (t *Token) SignDetached(key crypto.PrivateKey) (string, error) {
	jsonData, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	sig, err := jws.Sign(jsonData, jwa.EdDSA, key)
	if err != nil {
		return "", err
	}
	firstIndex := bytes.IndexByte(sig, '.')
	lastIndex := bytes.LastIndexByte(sig, '.')
	jsonDataNoSecret, err := json.Marshal(Token{
		ID: t.ID,
	})
	if err != nil {
		panic(err)
	}
	encodedPayloadNoSecret :=
		make([]byte, base64.RawURLEncoding.EncodedLen(len(jsonDataNoSecret)))
	base64.RawURLEncoding.Encode(encodedPayloadNoSecret, jsonDataNoSecret)
	buf := new(bytes.Buffer)
	buf.Write(sig[:firstIndex+1])
	buf.Write(encodedPayloadNoSecret)
	buf.Write(sig[lastIndex:])
	return string(buf.Bytes()), nil
}
