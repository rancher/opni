package b2mac_test

import (
	"encoding/base64"
	"regexp"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/b2mac"
)

var headerRegex = regexp.MustCompile(
	`(?i)^` + // case insensitive
		`MAC[\t\n\v\f\r ]+` + // authorization type
		`id[\t\n\v\f\r ]*=[\t\n\v\f\r ]*\"(?:[A-Za-z0-9_-]{4})*(?:[A-Za-z0-9_-]{2,3})?\",[\t\n\v\f\r ]*?` + // base64 RawUrlEncoding
		`nonce[\t\n\v\f\r ]*=[\t\n\v\f\r ]*\"[A-F0-9]{8}-[A-F0-9]{4}-4[A-F0-9]{3}-[89AB][A-F0-9]{3}-[A-F0-9]{12}\",[\t\n\v\f\r ]*?` + // uuidv4
		`mac[\t\n\v\f\r ]*=[\t\n\v\f\r ]*\"(?:[A-Za-z0-9_-]{4})*(?:[A-Za-z0-9_-]{2,3})?\"$`, // base64 RawUrlEncoding
)

var _ = Describe("Headers", Label("unit"), func() {
	DescribeTable("Encode Auth Headers",
		func(id []byte, nonce uuid.UUID, mac []byte, matchErr interface{}) {
			header, err := b2mac.EncodeAuthHeader(id, nonce, mac)
			if matchErr == nil {
				Expect(err).NotTo(HaveOccurred())
				Expect(headerRegex.MatchString(header)).To(BeTrue(), header)
			} else {
				Expect(err).To(MatchError(matchErr))
			}
		},
		Entry(nil, []byte("test"), uuid.New(), []byte("test"), nil),
		Entry(nil, []byte(""), uuid.New(), []byte(""), nil),
		Entry(nil, []byte(""), uuid.Nil, []byte(""), "nonce is not a v4 UUID"),
		Entry(nil, []byte(` !@#$%^&*()\-=_+[]{};':",./<>?|12345asdfg`), uuid.New(), []byte(` !@#$%^&*()\-=_+[]{};':",./<>?|12345asdfg`), nil),
	)
	var corruptInputErr base64.CorruptInputError
	DescribeTable("Decode Auth Headers",
		func(header string, id []byte, nonce uuid.UUID, mac []byte, matchErr interface{}) {
			i, n, m, err := b2mac.DecodeAuthHeader(header)
			if matchErr == nil {
				Expect(err).NotTo(HaveOccurred())
				Expect(i).To(Equal(id))
				Expect(n).To(Equal(nonce))
				Expect(m).To(Equal(mac))
			} else {
				Expect(err).To(MatchError(matchErr))
			}
		},

		Entry(nil, `MAC id="dGVzdA",nonce="5b8c6876-0c5f-4ee4-862f-0dd1fb29f771",mac="dGVzdA"`, []byte("test"), uuid.MustParse("5b8c6876-0c5f-4ee4-862f-0dd1fb29f771"), []byte("test"), nil),
		Entry(nil, `MAC nonce="5b8c6876-0c5f-4ee4-862f-0dd1fb29f771",mac="dGVzdA"`, []byte(""), uuid.Nil, []byte(""), "Header is missing id"),
		Entry(nil, `MAC id="dGVzdA",mac="dGVzdA"`, []byte(""), uuid.Nil, []byte(""), "Header is missing nonce"),
		Entry(nil, `MAC id="dGVzdA",nonce="5b8c6876-0c5f-4ee4-862f-0dd1fb29f771"`, []byte(""), uuid.Nil, []byte(""), "Header is missing signature"),
		Entry(nil, `Bearer XXXXXXXXXX`, []byte(""), uuid.Nil, []byte(""), "incorrect authorization type"),
		Entry(nil, `MAC id="dGVzdA",nonce="00000000_0000_0000_0000_000000000000",mac="dGVzdA"`, []byte(""), uuid.Nil, []byte(""), "invalid UUID format"),
		Entry(nil, `MAC id="dGVzdA",nonce="00000000-0000-0000-0000-000000000000",mac="dGVzdA"`, []byte(""), uuid.Nil, []byte(""), "nonce is not a v4 UUID"),
		Entry(nil, `MAC id="dGVzdA",nonce="5b8c6876-0c5f-4ee4-862f-0dd1fb29f771",mac="$$$$"`, []byte(""), uuid.Nil, []byte(""), corruptInputErr),
	)
})
