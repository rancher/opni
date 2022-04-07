package core_test

import (
	"unsafe"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni-monitoring/pkg/core"
)

var _ = Describe("Deep Copy", func() {
	It("should deep copy tokens", func() {
		token := &core.BootstrapToken{
			TokenID: "foo",
			Secret:  "bar",
			Metadata: &core.BootstrapTokenMetadata{
				LeaseID:    1234,
				Ttl:        5678,
				UsageCount: 9012,
				Labels: map[string]string{
					"foo": "bar",
				},
				Capabilities: []*core.TokenCapability{
					{
						Type: "test",
						Reference: &core.Reference{
							Id: "test",
						},
					},
				},
			},
		}

		tokenCopy1 := token.DeepCopy()
		tokenCopy2 := &core.BootstrapToken{}
		token.DeepCopyInto(tokenCopy2)

		Expect(token.TokenID).To(Equal(tokenCopy1.TokenID))
		Expect(token.Secret).To(Equal(tokenCopy1.Secret))
		Expect(uintptr(unsafe.Pointer(token.Metadata))).NotTo(Equal(uintptr(unsafe.Pointer(tokenCopy1.Metadata))))

		Expect(token.TokenID).To(Equal(tokenCopy2.TokenID))
		Expect(token.Secret).To(Equal(tokenCopy2.Secret))
		Expect(uintptr(unsafe.Pointer(token.Metadata))).NotTo(Equal(uintptr(unsafe.Pointer(tokenCopy2.Metadata))))
	})

	It("should deep copy clusters", func() {
		cluster := &core.Cluster{
			Id: "foo",
			Metadata: &core.ClusterMetadata{
				Labels: map[string]string{
					"foo": "bar",
				},
				Capabilities: []*core.ClusterCapability{
					{
						Name: "test",
					},
				},
			},
		}

		clusterCopy1 := cluster.DeepCopy()
		clusterCopy2 := &core.Cluster{}
		cluster.DeepCopyInto(clusterCopy2)

		Expect(cluster.Id).To(Equal(clusterCopy1.Id))
		Expect(uintptr(unsafe.Pointer(cluster.Metadata))).NotTo(Equal(uintptr(unsafe.Pointer(clusterCopy1.Metadata))))
		Expect(uintptr(unsafe.Pointer(cluster.Metadata.Capabilities[0]))).NotTo(Equal(uintptr(unsafe.Pointer(clusterCopy1.Metadata.Capabilities[0]))))

		Expect(cluster.Id).To(Equal(clusterCopy2.Id))
		Expect(uintptr(unsafe.Pointer(cluster.Metadata))).NotTo(Equal(uintptr(unsafe.Pointer(clusterCopy2.Metadata))))
		Expect(uintptr(unsafe.Pointer(cluster.Metadata.Capabilities[0]))).NotTo(Equal(uintptr(unsafe.Pointer(clusterCopy2.Metadata.Capabilities[0]))))
	})

	It("should deep copy roles", func() {
		role := &core.Role{
			Id:         "foo",
			ClusterIDs: []string{"foo"},
			MatchLabels: &core.LabelSelector{
				MatchLabels: map[string]string{"foo": "bar"},
				MatchExpressions: []*core.LabelSelectorRequirement{
					{
						Key:      "foo",
						Operator: "In",
						Values:   []string{"bar"},
					},
				},
			},
		}

		roleCopy1 := role.DeepCopy()
		roleCopy2 := &core.Role{}
		role.DeepCopyInto(roleCopy2)

		Expect(role.Id).To(Equal(roleCopy1.Id))
		Expect(role.ClusterIDs).To(Equal(roleCopy1.ClusterIDs))
		Expect(uintptr(unsafe.Pointer(role.MatchLabels))).NotTo(Equal(uintptr(unsafe.Pointer(roleCopy1.MatchLabels))))
		Expect(uintptr(unsafe.Pointer(role.MatchLabels.MatchExpressions[0]))).NotTo(Equal(uintptr(unsafe.Pointer(roleCopy1.MatchLabels.MatchExpressions[0]))))

		Expect(role.Id).To(Equal(roleCopy2.Id))
		Expect(role.ClusterIDs).To(Equal(roleCopy2.ClusterIDs))
		Expect(uintptr(unsafe.Pointer(role.MatchLabels))).NotTo(Equal(uintptr(unsafe.Pointer(roleCopy2.MatchLabels))))
	})

	It("should deep copy rolebindings", func() {
		roleBinding := &core.RoleBinding{
			Id:       "foo",
			RoleId:   "foo",
			Subjects: []string{"foo"},
			Taints:   []string{"foo"},
		}

		roleBindingCopy1 := roleBinding.DeepCopy()
		roleBindingCopy2 := &core.RoleBinding{}
		roleBinding.DeepCopyInto(roleBindingCopy2)

		Expect(roleBinding.Id).To(Equal(roleBindingCopy1.Id))
		Expect(roleBinding.RoleId).To(Equal(roleBindingCopy1.RoleId))
		Expect(roleBinding.Subjects).To(Equal(roleBindingCopy1.Subjects))
		Expect(roleBinding.Taints).To(Equal(roleBindingCopy1.Taints))

		Expect(roleBinding.Id).To(Equal(roleBindingCopy2.Id))
		Expect(roleBinding.RoleId).To(Equal(roleBindingCopy2.RoleId))
		Expect(roleBinding.Subjects).To(Equal(roleBindingCopy2.Subjects))
		Expect(roleBinding.Taints).To(Equal(roleBindingCopy2.Taints))
	})
})
