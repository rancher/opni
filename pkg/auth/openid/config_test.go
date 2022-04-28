package openid_test

import (
	"unsafe"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/auth/openid"
)

var _ = Describe("Config", Ordered, Label("unit"), func() {
	It("should properly check required fields", func() {
		cfg := openid.WellKnownConfiguration{}
		Expect(cfg.CheckRequiredFields()).To(MatchError(openid.ErrMissingRequiredField))
		cfg.Issuer = "foo"
		Expect(cfg.CheckRequiredFields()).To(MatchError(openid.ErrMissingRequiredField))
		cfg.AuthEndpoint = "foo"
		Expect(cfg.CheckRequiredFields()).To(MatchError(openid.ErrMissingRequiredField))
		cfg.TokenEndpoint = "foo"
		Expect(cfg.CheckRequiredFields()).To(MatchError(openid.ErrMissingRequiredField))
		cfg.UserinfoEndpoint = "foo"
		Expect(cfg.CheckRequiredFields()).To(MatchError(openid.ErrMissingRequiredField))
		cfg.JwksUri = "foo"
		Expect(cfg.CheckRequiredFields()).NotTo(HaveOccurred())
	})
	It("should deep copy", func() {
		cfg := &openid.OpenidConfig{
			Discovery:              &openid.DiscoverySpec{},
			WellKnownConfiguration: &openid.WellKnownConfiguration{},
		}
		cfg2 := cfg.DeepCopy()
		cfg3 := &openid.OpenidConfig{}
		cfg.DeepCopyInto(cfg3)

		Expect(uintptr(unsafe.Pointer(cfg.Discovery))).NotTo(Equal(uintptr(unsafe.Pointer(cfg2.Discovery))))
		Expect(uintptr(unsafe.Pointer(cfg.WellKnownConfiguration))).NotTo(Equal(uintptr(unsafe.Pointer(cfg2.WellKnownConfiguration))))
		Expect(uintptr(unsafe.Pointer(cfg.Discovery))).NotTo(Equal(uintptr(unsafe.Pointer(cfg3.Discovery))))
		Expect(uintptr(unsafe.Pointer(cfg.WellKnownConfiguration))).NotTo(Equal(uintptr(unsafe.Pointer(cfg3.WellKnownConfiguration))))
		Expect(uintptr(unsafe.Pointer(cfg2.Discovery))).NotTo(Equal(uintptr(unsafe.Pointer(cfg3.Discovery))))
		Expect(uintptr(unsafe.Pointer(cfg2.WellKnownConfiguration))).NotTo(Equal(uintptr(unsafe.Pointer(cfg3.WellKnownConfiguration))))
	})
})
