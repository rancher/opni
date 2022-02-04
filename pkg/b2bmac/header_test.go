package b2bmac_test

import (
	"encoding/base64"
	"regexp"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni-monitoring/pkg/b2bmac"
)

var headerRegex = regexp.MustCompile(
	`(?i)^` + // case insensitive
		`MAC ` + // authorization type
		`id=\".*?\",` + // arbitrary id string
		`nonce=\"[A-F0-9]{8}-[A-F0-9]{4}-4[A-F0-9]{3}-[89AB][A-F0-9]{3}-[A-F0-9]{12}\",` + // uuidv4
		`mac=\"(?:[A-Za-z0-9_-]{4})*(?:[A-Za-z0-9_-]{2,3})?\"$`, // base64 RawUrlEncoding
)

var _ = Describe("Headers", func() {
	DescribeTable("Encode Auth Headers",
		func(id string, nonce uuid.UUID, mac []byte, matchErr interface{}) {
			header, err := b2bmac.EncodeAuthHeader(id, nonce, mac)
			if matchErr == nil {
				Expect(err).NotTo(HaveOccurred())
				Expect(headerRegex.MatchString(header)).To(BeTrue(), header)
			} else {
				Expect(err).To(MatchError(matchErr))
			}
		},
		Entry(nil, "test", uuid.New(), []byte("test"), nil),
		Entry(nil, "", uuid.New(), []byte(""), nil),
		Entry(nil, "", uuid.Nil, []byte(""), "nonce is not a v4 UUID"),
		Entry(nil, ` !@#$%^&*()\-=_+[]{};':",./<>?|12345asdfg`, uuid.New(), []byte(` !@#$%^&*()\-=_+[]{};':",./<>?|12345asdfg`), nil),
	)
	var corruptInputErr base64.CorruptInputError
	DescribeTable("Decode Auth Headers",
		func(header string, id string, nonce uuid.UUID, mac []byte, matchErr interface{}) {
			i, n, m, err := b2bmac.DecodeAuthHeader(header)
			if matchErr == nil {
				Expect(err).NotTo(HaveOccurred())
				Expect(i).To(Equal(id))
				Expect(n).To(Equal(nonce))
				Expect(m).To(Equal(mac))
			} else {
				Expect(err).To(MatchError(matchErr))
			}
		},

		Entry(nil, `MAC id="test",nonce="5b8c6876-0c5f-4ee4-862f-0dd1fb29f771",mac="dGVzdA"`, "test", uuid.MustParse("5b8c6876-0c5f-4ee4-862f-0dd1fb29f771"), []byte("test"), nil),
		Entry(nil, `MAC nonce="5b8c6876-0c5f-4ee4-862f-0dd1fb29f771",mac="dGVzdA"`, "", uuid.Nil, []byte(""), "Header is missing id"),
		Entry(nil, `MAC id="test",mac="dGVzdA"`, "", uuid.Nil, []byte(""), "Header is missing nonce"),
		Entry(nil, `MAC id="test",nonce="5b8c6876-0c5f-4ee4-862f-0dd1fb29f771"`, "", uuid.Nil, []byte(""), "Header is missing signature"),
		Entry(nil, `Bearer XXXXXXXXXX`, "", uuid.Nil, []byte(""), "incorrect authorization type"),
		Entry(nil, `MAC id="test",nonce="00000000_0000_0000_0000_000000000000",mac="dGVzdA"`, "", uuid.Nil, []byte(""), "invalid UUID format"),
		Entry(nil, `MAC id="test",nonce="00000000-0000-0000-0000-000000000000",mac="dGVzdA"`, "", uuid.Nil, []byte(""), "nonce is not a v4 UUID"),
		Entry(nil, `MAC id="test",nonce="5b8c6876-0c5f-4ee4-862f-0dd1fb29f771",mac="$$$$"`, "", uuid.Nil, []byte(""), corruptInputErr),
	)
})
