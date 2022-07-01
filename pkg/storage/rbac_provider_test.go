package storage_test

import (
	"context"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test"
)

var _ = Describe("RBAC Provider", Ordered, Label("unit"), func() {
	clusters := []*corev1.Cluster{
		cluster("c1"),
		cluster("c2", "foo", "bar"),
		cluster("c3", "foo", "baz"),
		cluster("c4", "bar", "baz"),
		cluster("c5", "bar", "quux", "foo", "quux"),
	}
	entries := []TableEntry{
		Entry("1 role/1 cluster", rbacs(role("r1", "c1"), rb("rb1", "r1", "u1")), "u1", "c1"),
		Entry("1 role/2 clusters", rbacs(role("r1", "c1", "c2"), rb("rb1", "r1", "u1")), "u1", "c1", "c2"),
		Entry("2 roles/1 cluster", rbacs(role("r1", "c1"), role("r2", "c1"), rb("rb1", "r1", "u1")), "u1", "c1"),
		Entry("2 roles/2 clusters", rbacs(role("r1", "c1"), role("r2", "c2"), rb("rb1", "r1", "u1"), rb("rb2", "r2", "u1")), "u1", "c1", "c2"),
		Entry("0 roles", rbacs(), "u1"),
		Entry("binding with no role", rbacs(rb("rb1", "r1", "u1")), "u1"),
		Entry("binding with no cluster", rbacs(role("r1", "c1"), rb("rb1", "r1")), "u1"),
		Entry("role with no binding", rbacs(role("r1", "c1")), "u1"),
		Entry("role with no cluster", rbacs(role("r1"), rb("rb1", "r1", "u1")), "u1"),
		Entry("role with selector 1", rbacs(role("r1", matchLabels("foo", "bar")), rb("rb1", "r1", "u1")), "u1", "c2"),
		Entry("role with selector 2", rbacs(role("r1", matchLabels("foo", "baz")), rb("rb1", "r1", "u1")), "u1", "c3"),
		Entry("role with selector 3", rbacs(role("r1", matchLabels("baz", "quux")), rb("rb1", "r1", "u1")), "u1"),
		Entry("role with selector 4", rbacs(role("r1", matchExprs("foo Exists")), rb("rb1", "r1", "u1")), "u1", "c2", "c3", "c5"),
		Entry("role with selector 5", rbacs(role("r1", matchExprs("foo DoesNotExist")), rb("rb1", "r1", "u1")), "u1", "c1", "c4"),
		Entry("role with selector 6", rbacs(role("r1", matchExprs("foo In baz,quux")), rb("rb1", "r1", "u1")), "u1", "c3", "c5"),
		Entry("role with selector 7", rbacs(role("r1", matchExprs("foo NotIn xyz")), rb("rb1", "r1", "u1")), "u1", "c2", "c3", "c5"),
		Entry("role with selector 8", rbacs(role("r1", matchExprs("foo NotIn bar,baz")), rb("rb1", "r1", "u1")), "u1", "c5"),
		Entry("2 roles with 1 selector", rbacs(role("r1", matchExprs("foo Exists")), role("r2", matchExprs("bar Exists")), rb("rb1", "r1", "u1"), rb("rb2", "r2", "u1")), "u1", "c2", "c3", "c4", "c5"),
		Entry("1 role with 2 selectors", rbacs(role("r1", matchExprs("foo Exists", "bar Exists")), rb("rb1", "r1", "u1")), "u1", "c5"),
	}

	var ctrl *gomock.Controller
	BeforeAll(func() {
		ctrl = gomock.NewController(GinkgoT())
	})
	DescribeTable("Subject Access", func(objects rbacObjects, subject string, expected ...string) {
		rbacStore = test.NewTestRBACStore(ctrl)
		clusterStore := test.NewTestClusterStore(ctrl)
		for _, cluster := range clusters {
			err := clusterStore.CreateCluster(context.Background(), cluster)
			Expect(err).NotTo(HaveOccurred())
		}
		provider := storage.NewRBACProvider(struct {
			storage.RBACStore
			storage.ClusterStore
		}{
			RBACStore:    rbacStore,
			ClusterStore: clusterStore,
		})

		for _, obj := range objects.roles {
			err := rbacStore.CreateRole(context.Background(), obj())
			Expect(err).NotTo(HaveOccurred())
		}
		for _, obj := range objects.roleBindings {
			err := rbacStore.CreateRoleBinding(context.Background(), obj())
			Expect(err).NotTo(HaveOccurred())
		}
		refs, err := provider.SubjectAccess(context.Background(), &corev1.SubjectAccessRequest{
			Subject: subject,
		})
		Expect(err).NotTo(HaveOccurred())
		ids := make([]string, len(refs.Items))
		for i, ref := range refs.Items {
			ids[i] = ref.Id
		}
		Expect(ids).To(Equal(expected))
	}, entries)
})
