package core_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/core"
)

func tc(t string, id ...string) *core.TokenCapability {
	cap := &core.TokenCapability{
		Type: t,
	}
	if len(id) > 0 {
		cap.Reference = &core.Reference{
			Id: id[0],
		}
	}
	return cap
}

func cc(id string) *core.ClusterCapability {
	return &core.ClusterCapability{
		Name: id,
	}
}

var _ = Describe("Comparator", func() {
	DescribeTable("TokenCapability.Equal",
		func(a, b *core.TokenCapability, expected bool) {
			Expect(core.Comparator[*core.TokenCapability].Equal(a, b)).To(Equal(expected))
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
		func(a, b *core.ClusterCapability, expected bool) {
			Expect(core.Comparator[*core.ClusterCapability].Equal(a, b)).To(Equal(expected))
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
