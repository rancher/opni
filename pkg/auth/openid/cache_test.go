package openid_test

import (
	"fmt"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
	"go.uber.org/atomic"

	"github.com/rancher/opni/pkg/auth/openid"
	"github.com/rancher/opni/pkg/test"
)

var _ = Describe("User Info Cache", Ordered, Label("unit"), func() {
	var cache *openid.UserInfoCache
	lg := test.Log
	var addr string
	requestCount := atomic.NewInt32(0)
	userMap := map[string]string{} // token: sub

	BeforeAll(func() {
		port, err := freeport.GetFreePort()
		Expect(err).NotTo(HaveOccurred())
		addr = fmt.Sprintf("localhost:%d", port)
		mux := http.NewServeMux()
		mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
			requestCount.Inc()
			token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			w.Write([]byte(fmt.Sprintf(`{"sub":%q}`, userMap[token])))
		})
		srv := http.Server{
			Addr:    addr,
			Handler: mux,
		}
		go srv.ListenAndServe()
		DeferCleanup(srv.Close)
	})

	BeforeEach(func() {
		userMap = map[string]string{}
		requestCount.Store(0)
		cache, _ = openid.NewUserInfoCache(&openid.OpenidConfig{
			WellKnownConfiguration: &openid.WellKnownConfiguration{
				UserinfoEndpoint: "http://" + addr + "/userinfo",
			},
			IdentifyingClaim: "sub",
		}, lg)
	})

	When("an access token is requested", func() {
		It("should fetch user info from the userinfo endpoint", func() {
			userMap["foo"] = "test"
			info, err := cache.Get("foo")
			Expect(err).NotTo(HaveOccurred())
			userID, err := info.UserID()
			Expect(err).NotTo(HaveOccurred())
			Expect(userID).To(Equal("test"))
			Expect(requestCount.Load()).To(Equal(int32(1)))
		})
	})
	When("an access token is requested multiple times", func() {
		It("should fetch user info from the userinfo endpoint only once", func() {
			userMap["foo"] = "test"
			for i := 0; i < 100; i++ {
				info, err := cache.Get("foo")
				Expect(err).NotTo(HaveOccurred())
				userID, err := info.UserID()
				Expect(err).NotTo(HaveOccurred())
				Expect(userID).To(Equal("test"))
				Expect(requestCount.Load()).To(Equal(int32(1)))
			}
		})
	})
	When("multiple access tokens are requested", func() {
		It("should cache each individually", func() {
			userMap["foo"] = "test"
			userMap["bar"] = "test2"

			for i := 0; i < 10; i++ {
				info, err := cache.Get("foo")
				Expect(err).NotTo(HaveOccurred())
				userID, err := info.UserID()
				Expect(err).NotTo(HaveOccurred())
				Expect(userID).To(Equal("test"))
				Expect(requestCount.Load()).To(Equal(int32(1)))
			}

			for i := 0; i < 10; i++ {
				info, err := cache.Get("bar")
				Expect(err).NotTo(HaveOccurred())
				userID, err := info.UserID()
				Expect(err).NotTo(HaveOccurred())
				Expect(userID).To(Equal("test2"))
				Expect(requestCount.Load()).To(Equal(int32(2)))
			}

			for i := 0; i < 10; i++ {
				info, err := cache.Get("foo")
				Expect(err).NotTo(HaveOccurred())
				userID, err := info.UserID()
				Expect(err).NotTo(HaveOccurred())
				Expect(userID).To(Equal("test"))
				Expect(requestCount.Load()).To(Equal(int32(2)))
			}
		})
	})

	When("a user's access token is refreshed", func() {
		It("should invalidate the old token", func() {
			userMap["foo"] = "test"
			info, err := cache.Get("foo")
			Expect(err).NotTo(HaveOccurred())
			userID, err := info.UserID()
			Expect(err).NotTo(HaveOccurred())
			Expect(userID).To(Equal("test"))
			Expect(requestCount.Load()).To(Equal(int32(1)))

			userMap["bar"] = "test"
			info, err = cache.Get("bar")
			Expect(err).NotTo(HaveOccurred())
			userID, err = info.UserID()
			Expect(err).NotTo(HaveOccurred())
			Expect(userID).To(Equal("test"))
			Expect(requestCount.Load()).To(Equal(int32(2)))
		})
	})

	Context("error handling", func() {
		When("the userinfo endpoint returns an error", func() {
			It("should return an error", func() {
				cache, err := openid.NewUserInfoCache(&openid.OpenidConfig{
					WellKnownConfiguration: &openid.WellKnownConfiguration{
						UserinfoEndpoint: "http://" + addr + "/userinfo2",
					},
					IdentifyingClaim: "sub",
				}, lg)
				Expect(err).NotTo(HaveOccurred())
				_, err = cache.Get("foo")
				Expect(err).To(MatchError("404 Not Found"))

				cache, err = openid.NewUserInfoCache(&openid.OpenidConfig{
					WellKnownConfiguration: &openid.WellKnownConfiguration{
						UserinfoEndpoint: "http://" + addr + "/userinfo",
					},
					IdentifyingClaim: "email",
				}, lg)
				Expect(err).NotTo(HaveOccurred())
				_, err = cache.Get("foo")
				Expect(err).To(MatchError(`identifying claim "email" not found in user info`))
			})
		})
		When("the cache is configured improperly", func() {
			It("should return an error", func() {
				By("ensuring IdentifyingClaim is set")
				_, err := openid.NewUserInfoCache(&openid.OpenidConfig{
					WellKnownConfiguration: &openid.WellKnownConfiguration{
						UserinfoEndpoint: "http://" + addr + "/userinfo",
					},
				}, lg)
				Expect(err).To(MatchError("no identifying claim set"))

				By("ensuring WellKnownConfiguration or Discovery is set", func() {
					_, err := openid.NewUserInfoCache(&openid.OpenidConfig{
						IdentifyingClaim: "sub",
					}, lg)
					Expect(err).To(MatchError(openid.ErrMissingDiscoveryConfig))
				})
			})
		})
	})
})
