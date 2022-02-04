package rbac_test

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/atomic"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/rbac"
	mock_rbac "github.com/rancher/opni-monitoring/pkg/test/mock/rbac"
)

var testUsers = map[string][]string{
	"user0": {"tenant1", "tenant2"},
	"user1": {"tenant1"},
	"user2": {"tenant2"},
	"user3": {},
}

var _ = Describe("Middleware", func() {
	It("should set tenant IDs for authorized users", func() {
		By("setting up the test controller")
		ctrl := gomock.NewController(GinkgoT())
		mockProvider := mock_rbac.NewMockProvider(ctrl)
		mockProvider.EXPECT().
			SubjectAccess(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, sar *core.SubjectAccessRequest) (*core.ReferenceList, error) {
				if clusters, ok := testUsers[sar.Subject]; ok {
					items := make([]*core.Reference, len(clusters))
					for i, cluster := range clusters {
						items[i] = &core.Reference{
							Id: cluster,
						}
					}
					return &core.ReferenceList{
						Items: items,
					}, nil
				}
				return nil, errors.New("user not found")
			}).
			AnyTimes()
		defer ctrl.Finish()
		app := fiber.New()
		logger.ConfigureApp(app, logger.New().Named("test"))

		By("adding test middleware to insert the userID local")
		id := atomic.NewInt32(0)
		app.Use(func(c *fiber.Ctx) error {
			num := id.Load()
			c.Locals(rbac.UserIDKey, fmt.Sprintf("user%d", num))
			id.Store((num + 1) % 5) // includes nonexistent "user4"
			return c.Next()
		})

		By("adding the rbac middleware")
		app.Use(rbac.NewMiddleware(mockProvider))

		By("adding test middleware to check the resulting headers")
		app.Use(func(c *fiber.Ctx) error {
			defer GinkgoRecover()
			// This middleware should only be hit if the user is authorized
			userId := c.Locals(rbac.UserIDKey)
			Expect(userId).NotTo(BeNil())
			req := c.Request()
			orgId := string(req.Header.Peek("X-Scope-OrgID"))
			Expect(orgId).NotTo(BeEmpty())
			tenants := testUsers[userId.(string)]
			Expect(tenants).NotTo(BeEmpty())
			Expect(orgId).To(Equal(strings.Join(tenants, "|")))
			return c.Next()
		})

		By("adding a default 200 handler")
		app.Get("/", func(c *fiber.Ctx) error {
			return c.SendStatus(fiber.StatusOK)
		})

		By("checking request status codes")
		for i := 0; i < 50; i++ {
			idNum := id.Load() // order is important - this gets modified in the handler
			userId := fmt.Sprintf("user%d", idNum)
			resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
			Expect(err).NotTo(HaveOccurred())
			if idNum > 2 {
				Expect(resp.StatusCode).To(Equal(fiber.StatusUnauthorized), userId)
			} else {
				Expect(resp.StatusCode).To(Equal(fiber.StatusOK), userId)
			}
		}
	})
	It("should skip the middleware if no user ID key is set", func() {
		By("setting up the test controller")
		ctrl := gomock.NewController(GinkgoT())
		mockProvider := mock_rbac.NewMockProvider(ctrl)
		mockProvider.EXPECT().
			SubjectAccess(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, sar *core.SubjectAccessRequest) ([]string, error) {
				defer GinkgoRecover()
				Fail("this should not be called")
				return nil, nil
			}).
			AnyTimes()
		defer ctrl.Finish()
		app := fiber.New()
		logger.ConfigureApp(app, logger.New().Named("test"))

		By("adding the rbac middleware")
		app.Use(rbac.NewMiddleware(mockProvider))

		By("adding a default 200 handler")
		app.Get("/", func(c *fiber.Ctx) error {
			return c.SendStatus(fiber.StatusOK)
		})

		By("checking request status codes")
		for i := 0; i < 50; i++ {
			resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(fiber.StatusOK))
		}
	})
})
