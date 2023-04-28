package storage_test

import (
	"context"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test/mock/storage"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
)

var _ = Describe("Taints", Ordered, Label("unit"), func() {
	var ctrl *gomock.Controller
	BeforeAll(func() {
		ctrl = gomock.NewController(GinkgoT())
	})
	When("A referenced role is missing", func() {
		It("should apply the relevant taint", func() {
			store := mock_storage.NewTestRBACStore(ctrl)
			rb := &corev1.RoleBinding{
				Id:       "test",
				RoleId:   "test",
				Subjects: []string{"foo"},
			}
			err := storage.ApplyRoleBindingTaints(context.Background(), store, rb)
			Expect(err).NotTo(HaveOccurred())
			Expect(rb.Taints).To(Equal([]string{"role not found"}))

			err = store.CreateRole(context.Background(), &corev1.Role{
				Id:         "test",
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
			store := mock_storage.NewTestRBACStore(ctrl)
			err := store.CreateRole(context.Background(), &corev1.Role{
				Id:         "test",
				ClusterIDs: []string{"foo"},
			})
			Expect(err).NotTo(HaveOccurred())

			rb := &corev1.RoleBinding{
				Id:       "test",
				RoleId:   "test",
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
