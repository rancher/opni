package ident_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/ident"
)

type testProvider struct{}

func (tp *testProvider) UniqueIdentifier(context.Context) (string, error) {
	return "foo", nil
}

func newTestProvider() ident.Provider {
	return &testProvider{}
}

var _ = Describe("Ident", Label("unit"), func() {
	When("registering a new provider", func() {
		It("should succeed if the provider does not exist yet", func() {
			Expect(ident.RegisterProvider("test1", newTestProvider)).To(Succeed())
		})
		It("should return an error if the provider name is empty", func() {
			Expect(ident.RegisterProvider("", newTestProvider)).To(MatchError(ident.ErrInvalidProviderName))
		})
		It("should return an error if the provider already exists", func() {
			ident.RegisterProvider("test2", newTestProvider)
			Expect(ident.RegisterProvider("test2", newTestProvider)).To(MatchError(ident.ErrProviderAlreadyExists))
		})
		It("should return an error if the provider is nil", func() {
			Expect(ident.RegisterProvider("test3", nil)).To(MatchError(ident.ErrNilProvider))
		})
	})

	When("getting a provider object by name", func() {
		It("should return an error if the provider is not found", func() {
			_, err := ident.GetProvider("test4")
			Expect(err).To(MatchError(ident.ErrProviderNotFound))
		})
		It("should return the provider object if it is found", func() {
			ident.RegisterProvider("test5", newTestProvider)
			mw, err := ident.GetProvider("test5")
			Expect(err).NotTo(HaveOccurred())
			Expect(mw.Name()).To(Equal("test5"))
		})
	})
})
