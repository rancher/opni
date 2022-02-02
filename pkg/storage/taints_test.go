package storage_test

import (
	"context"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kralicky/opni-monitoring/pkg/core"
	"github.com/kralicky/opni-monitoring/pkg/storage"
	"github.com/kralicky/opni-monitoring/pkg/test"
)

var _ = Describe("Taints", Ordered, func() {
	var ctrl *gomock.Controller
	BeforeAll(func() {
		ctrl = gomock.NewController(GinkgoT())
	})
	When("A referenced role is missing", func() {
		It("should apply the relevant taint", func() {
			store := test.NewTestRBACStore(ctrl)
			rb := &core.RoleBinding{
				Name:     "test",
				RoleName: "test",
				Subjects: []string{"foo"},
			}
			err := storage.ApplyRoleBindingTaints(context.Background(), store, rb)
			Expect(err).NotTo(HaveOccurred())
			Expect(rb.Taints).To(Equal([]string{"role not found"}))

			err = store.CreateRole(context.Background(), &core.Role{
				Name:       "test",
				ClusterIDs: []string{"foo"},
			})
			Expect(err).NotTo(HaveOccurred())

			rb.Taints = []string{}
			err = storage.ApplyRoleBindingTaints(context.Background(), store, rb)
			Expect(err).NotTo(HaveOccurred())
			Expect(rb.Taints).To(BeEmpty())
		})
	})
	When("A role binding has no subjects", func() {
		It("should apply the relevant taint", func() {
			store := test.NewTestRBACStore(ctrl)
			err := store.CreateRole(context.Background(), &core.Role{
				Name:       "test",
				ClusterIDs: []string{"foo"},
			})
			Expect(err).NotTo(HaveOccurred())

			rb := &core.RoleBinding{
				Name:     "test",
				RoleName: "test",
				Subjects: []string{},
			}
			err = storage.ApplyRoleBindingTaints(context.Background(), store, rb)
			Expect(err).NotTo(HaveOccurred())
			Expect(rb.Taints).To(Equal([]string{"no subjects"}))

			rb.Subjects = []string{"foo"}
			rb.Taints = []string{}
			err = storage.ApplyRoleBindingTaints(context.Background(), store, rb)
			Expect(err).NotTo(HaveOccurred())
			Expect(rb.Taints).To(BeEmpty())
		})
	})
})
