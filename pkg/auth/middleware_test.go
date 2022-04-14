package auth_test

import (
	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/auth"
)

type testMiddleware struct{}

func (tm *testMiddleware) Handle(ctx *fiber.Ctx) error {
	return nil
}

var _ = Describe("Middleware", func() {
	AfterEach(func() {
		auth.ResetMiddlewares()
	})
	When("registering a new middleware object", func() {
		It("should succeed if the middleware does not exist yet", func() {
			Expect(auth.RegisterMiddleware("test", &testMiddleware{})).To(Succeed())
		})
		It("should return an error if the middleware name is empty", func() {
			Expect(auth.RegisterMiddleware("", &testMiddleware{})).To(MatchError(auth.ErrInvalidMiddlewareName))
		})
		It("should return an error if the middleware already exists", func() {
			auth.RegisterMiddleware("test", &testMiddleware{})
			Expect(auth.RegisterMiddleware("test", &testMiddleware{})).To(MatchError(auth.ErrMiddlewareAlreadyExists))
		})
		It("should return an error if the middleware is nil", func() {
			Expect(auth.RegisterMiddleware("test", nil)).To(MatchError(auth.ErrNilMiddleware))
		})
	})

	When("getting a middleware object by name", func() {
		It("should return an error if the middleware is not found", func() {
			_, err := auth.GetMiddleware("test")
			Expect(err).To(MatchError(auth.ErrMiddlewareNotFound))
		})
		It("should return the middleware object if it is found", func() {
			auth.RegisterMiddleware("test", &testMiddleware{})
			mw, err := auth.GetMiddleware("test")
			Expect(err).NotTo(HaveOccurred())
			Expect(mw.Name()).To(Equal("test"))

			tm := auth.NamedMiddlewareAs[*testMiddleware](mw)
			Expect(tm).NotTo(BeNil())
		})
	})
})
