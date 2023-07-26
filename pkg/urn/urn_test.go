package urn_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/urn"
)

var _ = Describe("URN", Label("unit"), func() {
	When("URN doesn't start with urn", func() {
		It("should return error", func() {
			_, err := urn.ParseString("noturn:foo:bar:baz:bat")
			Expect(err).To(MatchError(urn.ErrInvalidURN))
		})
	})
	When("URN doesn't have 5 parts", func() {
		It("should return error", func() {
			_, err := urn.ParseString("urn:foo:bar:baz")
			Expect(err).To(MatchError(urn.ErrInvalidURN))
			_, err = urn.ParseString("urn:foo:bar:baz:bat:qux")
			Expect(err).To(MatchError(urn.ErrInvalidURN))
		})
	})
	When("URN namespace is not opni", func() {
		It("should return error", func() {
			_, err := urn.ParseString("urn:foo:bar:baz:bat")
			Expect(err).To(MatchError(urn.ErrInvalidURN))
			Expect(err).To(MatchError(ContainSubstring("invalid namespace: foo")))
		})
	})
	When("URN is valid", func() {
		It("should parse successfully", func() {
			u, err := urn.ParseString("urn:opni:plugin:foo:bar")
			Expect(err).NotTo(HaveOccurred())
			Expect(u.Namespace).To(Equal("opni"))
			Expect(u.Type).To(Equal(urn.Plugin))
			Expect(u.Strategy).To(Equal("foo"))
			Expect(u.Component).To(Equal("bar"))
		})
	})
	Context("String construction", func() {
		Specify("should return a correct string", func() {
			u := urn.NewOpniURN(urn.Agent, "foo", "bar")
			Expect(u.String()).To(Equal("urn:opni:agent:foo:bar"))
		})
	})
})
