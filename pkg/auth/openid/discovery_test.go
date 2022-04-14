package openid_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/auth/openid"
	"github.com/rancher/opni/pkg/util"
)

var _ = Describe("Discovery", Ordered, Label("unit"), func() {
	It("should query the discovery endpoint if configured", func() {
		cfg := &openid.OpenidConfig{
			Discovery: &openid.DiscoverySpec{
				Issuer: "http://" + discovery.addr,
			},
		}
		response, err := cfg.GetWellKnownConfiguration()
		Expect(err).NotTo(HaveOccurred())
		Expect(*response).To(Equal(discovery.wellKnownCfg))
	})

	It("should not redirect", func() {
		cfg := &openid.OpenidConfig{
			Discovery: &openid.DiscoverySpec{
				Path:   util.Pointer("/bad-redirect-test"),
				Issuer: "http://" + discovery.addr,
			},
		}
		_, err := cfg.GetWellKnownConfiguration()
		Expect(err).To(MatchError("failed to fetch configuration from discovery endpoint: 301 Moved Permanently"))
	})

	Context("error handling", func() {
		When("the issuer URL is invalid", func() {
			It("should error", func() {
				cfg := &openid.OpenidConfig{
					Discovery: &openid.DiscoverySpec{
						Issuer: "http://%%%%%%",
					},
				}
				_, err := cfg.GetWellKnownConfiguration()
				var urlError url.Error
				Expect(err).To(HaveOccurred())
				Expect(errors.Unwrap(err)).To(BeAssignableToTypeOf(&urlError))
			})
		})
		When("the issuer URL does not match the expected URL", func() {
			It("should error", func() {
				cfg := &openid.OpenidConfig{
					Discovery: &openid.DiscoverySpec{
						Issuer: "http://" + discovery.addr + "/",
					},
				}
				_, err := cfg.GetWellKnownConfiguration()
				Expect(err).To(MatchError(openid.ErrIssuerMismatch))
			})
		})
		When("the server returns invalid discovery data", func() {
			It("should error", func() {
				By("checking for HTTP errors")
				cfg := &openid.OpenidConfig{
					Discovery: &openid.DiscoverySpec{
						Path:   util.Pointer("/bad-response-code-test"),
						Issuer: "http://" + discovery.addr,
					},
				}
				_, err := cfg.GetWellKnownConfiguration()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(http.StatusText(http.StatusInternalServerError)))

				By("checking for invalid JSON")
				cfg = &openid.OpenidConfig{
					Discovery: &openid.DiscoverySpec{
						Path:   util.Pointer("/bad-json-test"),
						Issuer: "http://" + discovery.addr,
					},
				}
				_, err = cfg.GetWellKnownConfiguration()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unexpected EOF"))

				By("checking for missing fields")
				cfg = &openid.OpenidConfig{
					Discovery: &openid.DiscoverySpec{
						Path:   util.Pointer("/missing-fields-test"),
						Issuer: "http://" + discovery.addr,
					},
				}
				_, err = cfg.GetWellKnownConfiguration()
				Expect(err).To(MatchError(openid.ErrMissingRequiredField))

				By("timing out if the server takes too long")
				cfg = &openid.OpenidConfig{
					Discovery: &openid.DiscoverySpec{
						Path:   util.Pointer("/timeout"),
						Issuer: "http://" + discovery.addr,
					},
				}
				_, err = cfg.GetWellKnownConfiguration()
				Expect(err).To(MatchError(context.DeadlineExceeded))
			})
		})
	})
})
