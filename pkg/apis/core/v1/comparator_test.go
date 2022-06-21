package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "github.com/rancher/opni/pkg/apis/core/v1"
)

func tc(t string, id ...string) *v1.TokenCapability {
	cap := &v1.TokenCapability{
		Type: t,
	}
	if len(id) > 0 {
		cap.Reference = &v1.Reference{
			Id: id[0],
		}
	}
	return cap
}

func cc(id string) *v1.ClusterCapability {
	return &v1.ClusterCapability{
		Name: id,
	}
}

var _ = Describe("Comparator", Label("unit"), func() {
	DescribeTable("TokenCapability.Equal",
		func(a, b *v1.TokenCapability, expected bool) {
			Expect(v1.Comparator[*v1.TokenCapability].Equal(a, b)).To(Equal(expected))
		},
		Entry(nil, tc("cap1", "id1"), tc("cap1", "id1"), true),
		Entry(nil, tc("cap1", "id1"), tc("cap2", "id1"), false),
		Entry(nil, tc("cap1", "id1"), tc("cap1", "id2"), false),
		Entry(nil, tc("", ""), tc("", ""), false),
		Entry(nil, tc(""), tc("", ""), false),
		Entry(nil, tc("", ""), tc("cap1", "id1"), false),
		Entry(nil, tc(""), tc("cap1", "id1"), false),
		Entry(nil, tc("cap1", "id1"), tc("", ""), false),
		Entry(nil, tc("cap1", ""), tc("cap1", "id1"), false),
		Entry(nil, tc("cap1", ""), tc("cap1"), false),
		Entry(nil, tc("cap1"), tc("cap1"), false),
		Entry(nil, tc(""), tc(""), false),
		Entry(nil, nil, tc("cap1", "id1"), false),
		Entry(nil, tc("cap1", "id1"), nil, false),
		Entry(nil, nil, nil, false),
	)

	DescribeTable("ClusterCapability.Equal",
		func(a, b *v1.ClusterCapability, expected bool) {
			Expect(v1.Comparator[*v1.ClusterCapability].Equal(a, b)).To(Equal(expected))
		},
		Entry(nil, cc("id1"), cc("id1"), true),
		Entry(nil, cc("id1"), cc("id2"), false),
		Entry(nil, cc(""), cc(""), false),
		Entry(nil, cc(""), cc("id1"), false),
		Entry(nil, cc("id1"), cc(""), false),
		Entry(nil, cc(""), cc(""), false),
		Entry(nil, nil, cc("id1"), false),
		Entry(nil, cc("id1"), nil, false),
		Entry(nil, nil, nil, false),
	)
})
