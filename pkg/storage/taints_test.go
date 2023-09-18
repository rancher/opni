package storage_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mock_storage "github.com/rancher/opni/pkg/test/mock/storage"
	"go.uber.org/mock/gomock"

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
				Id:      "test",
				RoleIds: []string{"test"},
				Subject: "foo",
			}
			err := storage.ApplyRoleBindingTaints(context.Background(), store, rb)
			Expect(err).NotTo(HaveOccurred())
			Expect(rb.Taints).To(Equal([]string{"role test not found"}))

			err = store.CreateRole(context.Background(), &corev1.Role{
				Id: "test",
				Permissions: []*corev1.PermissionItem{
					{
						Type: string(corev1.PermissionTypeCluster),
						Verbs: []*corev1.PermissionVerb{
							{Verb: string(storage.ClusterVerbGet)},
						},
						Ids: []string{"foo"},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			rb.Taints = []string{}
			err = storage.ApplyRoleBindingTaints(context.Background(), store, rb)
			Expect(err).NotTo(HaveOccurred())
			Expect(rb.Taints).To(BeEmpty())
		})
		It("should only apply the relevant taint once", func() {
			store := mock_storage.NewTestRBACStore(ctrl)
			rb := &corev1.RoleBinding{
				Id:      "test-rb2",
				RoleIds: []string{"does-not-exist"},
				Subject: "foo",
			}
			err := storage.ApplyRoleBindingTaints(context.Background(), store, rb)
			Expect(err).NotTo(HaveOccurred())
			err = storage.ApplyRoleBindingTaints(context.Background(), store, rb)
			Expect(err).NotTo(HaveOccurred())
			Expect(rb.Taints).To(Equal([]string{"role does-not-exist not found"}))
		})
	})
	When("A role binding has no subjects", func() {
		It("should apply the relevant taint", func() {
			store := mock_storage.NewTestRBACStore(ctrl)
			err := store.CreateRole(context.Background(), &corev1.Role{
				Id: "test",
				Permissions: []*corev1.PermissionItem{
					{
						Type: string(corev1.PermissionTypeCluster),
						Verbs: []*corev1.PermissionVerb{
							{Verb: string(storage.ClusterVerbGet)},
						},
						Ids: []string{"foo"},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			rb := &corev1.RoleBinding{
				Id:      "test",
				RoleIds: []string{"test"},
				Subject: "",
			}
			err = storage.ApplyRoleBindingTaints(context.Background(), store, rb)
			Expect(err).NotTo(HaveOccurred())
			Expect(rb.Taints).To(Equal([]string{"no subjects"}))

			rb.Subject = "foo"
			rb.Taints = []string{}
			err = storage.ApplyRoleBindingTaints(context.Background(), store, rb)
			Expect(err).NotTo(HaveOccurred())
			Expect(rb.Taints).To(BeEmpty())
		})
	})
})
