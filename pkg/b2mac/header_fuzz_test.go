package b2mac_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rancher/opni/pkg/b2mac"
)

func FuzzDecodeAuthHeader(f *testing.F) {
	entries := []string{
		`MAC id="test",nonce="5b8c6876-0c5f-4ee4-862f-0dd1fb29f771",mac="dGVzdA"`,
		`MAC nonce="5b8c6876-0c5f-4ee4-862f-0dd1fb29f771",mac="dGVzdA"`,
		`MAC id="test",mac="dGVzdA"`,
		`MAC id="test",nonce="5b8c6876-0c5f-4ee4-862f-0dd1fb29f771"`,
		`Bearer XXXXXXXXXX`,
		`MAC id="test",nonce="00000000_0000_0000_0000_000000000000",mac="dGVzdA"`,
		`MAC id="test",nonce="00000000-0000-0000-0000-000000000000",mac="dGVzdA"`,
		`MAC id="test",nonce="5b8c6876-0c5f-4ee4-862f-0dd1fb29f771",mac="$$$$"`,

		"MAC id =\"00\",nonce=\"00000000-0000-4000-8000-000000000000\",mac=\"00\"",
		"MAC id=\"00\",nonce=\"00000000-0000-4000-8000-000000000000\",mac=\"\r00\"",
		"MAC id=\"\x80\",nonce=\"00000000-0000-4000-0000-000000000000\",mac=\"00\"",
		"MAC id=\"00\",nonce=\"00000000-0000-4000-8000-000000000000\",mac=\"\",mac=\"00\"",
		"MAC nonce",
		"MAC  id=\"0\",nonce=\"00000000-0000-4000-8000-000000000000\",mac=\"00\"",
		"MAC id=\"00\",nonce=\"00000000-0000-4000-8000-000000000000\", mac=\"00\"",
		"MAC id=\"test\",nonce=\"5b8c6876-dc5f-0000-8000-000000000000\",mac=\"000000\"",
		"MAC id\v=\"00\",nonce=\"00000000-0000-4000-8000-000000000000\",mac=\"00\"",
		"MAC id=\"0\",nonce=\"00000000-0000-4000-8000-000000000000\",=\"\",mac=\"00\"",
		"MAC id=0,nonce=00000000-0000-0000-8000-000000000000,mac=",
		"MAC id=\"0\r0\",nonce=\"00000000-0000-4000-8000-000000000000\",mac=\"00\"",
		"MAC id=\"0\",nonce=\"00000000-0000-0000-8000-000000000000\",mac=\"\"",
		"MAC id=\"\n\",nonce=\"00000000-0000-4000-8000-000000000000\",mac=\"00\"",
	}
	for _, entry := range entries {
		f.Add(entry)
	}
	f.Fuzz(func(t *testing.T, header string) {
		id, nonce, mac, err := b2mac.DecodeAuthHeader(header)
		if err == nil {
			// \r and \n are silently ignored in the base64 decoding algorithm, so
			// remove any of these characters from the input
			header = strings.ReplaceAll(header, "\r", "")
			header = strings.ReplaceAll(header, "\n", "")

			if ok := headerRegex.MatchString(header); !ok {
				t.Errorf("decoded header does not match expected regex")
			}

			str, err := b2mac.EncodeAuthHeader(id, nonce, mac)
			if err != nil {
				t.Errorf("re-encoded header failed: %v", err)
			}
			id2, nonce2, mac2, err2 := b2mac.DecodeAuthHeader(str)
			if err2 != nil {
				t.Errorf("decoding re-encoded header failed: %v", err2)
			}
			if !bytes.Equal(id, id2) {
				t.Errorf("id mismatch: %v != %v", id, id2)
			}
			if nonce != nonce2 {
				t.Errorf("nonce mismatch: %v != %v", nonce, nonce2)
			}
			if !bytes.Equal(mac, mac2) {
				t.Errorf("mac mismatch: %v != %v", mac, mac2)
			}
		}
	})
}
