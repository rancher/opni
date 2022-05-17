package core_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/core"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/validation"
)

func validateEntry[T validation.Validator](in T, expected error) {
	if expected == nil {
		Expect(in.Validate()).To(Succeed())
	} else {
		Expect(in.Validate()).To(MatchError(expected))
	}
}

var _ = Describe("Validation", Label(test.Unit), func() {
	DescribeTable("Cluster", validateEntry[*core.Cluster],
		Entry(nil, &core.Cluster{Id: "foo"}, nil),
		Entry(nil, &core.Cluster{}, validation.ErrMissingRequiredField),
		Entry(nil, &core.Cluster{Id: "$"}, validation.ErrInvalidID),
		Entry(nil, &core.Cluster{
			Id: "foo",
			Metadata: &core.ClusterMetadata{
				Labels: map[string]string{"\\": "foo"},
			},
		}, validation.ErrInvalidLabelName),
	)
	DescribeTable("LabelSelector", validateEntry[*core.LabelSelector],
		Entry(nil, &core.LabelSelector{}, nil),
		Entry(nil, &core.LabelSelector{
			MatchLabels: map[string]string{"foo": "\\"},
		}, validation.ErrInvalidLabelValue),
		Entry(nil, &core.LabelSelector{
			MatchLabels: map[string]string{"foo": "bar"},
			MatchExpressions: []*core.LabelSelectorRequirement{
				{Key: "foo", Operator: "invalid"},
			},
		}, validation.ErrInvalidValue),
	)
	DescribeTable("LabelSelectorRequirement", validateEntry[*core.LabelSelectorRequirement],
		Entry(nil, &core.LabelSelectorRequirement{}, validation.ErrMissingRequiredField),
		Entry(nil, &core.LabelSelectorRequirement{Key: "foo"}, validation.ErrMissingRequiredField),
		Entry(nil, &core.LabelSelectorRequirement{
			Key:      "\\",
			Operator: string(core.LabelSelectorOpExists),
		}, validation.ErrInvalidLabelName),
		Entry(nil, &core.LabelSelectorRequirement{
			Key:      "foo",
			Operator: "invalid",
		}, validation.ErrInvalidValue),
		Entry(nil, &core.LabelSelectorRequirement{
			Key:      "foo",
			Operator: string(core.LabelSelectorOpExists),
			Values:   []string{"\\"},
		}, validation.ErrInvalidLabelValue),
		Entry(nil, &core.LabelSelectorRequirement{
			Key:      "foo",
			Operator: string(core.LabelSelectorOpExists),
			Values:   []string{"bar"},
		}, nil),
	)
	DescribeTable("Role", validateEntry[*core.Role],
		Entry(nil, &core.Role{}, validation.ErrMissingRequiredField),
		Entry(nil, &core.Role{Id: "\\"}, validation.ErrInvalidID),
		Entry(nil, &core.Role{
			Id:         "foo",
			ClusterIDs: []string{"\\"},
		}, validation.ErrInvalidID),
		Entry(nil, &core.Role{
			Id:         "foo",
			ClusterIDs: []string{"bar"},
			MatchLabels: &core.LabelSelector{
				MatchExpressions: []*core.LabelSelectorRequirement{
					{Key: "foo", Operator: "invalid"},
				},
			},
		}, validation.ErrInvalidValue),
		Entry(nil, &core.Role{
			Id:         "foo",
			ClusterIDs: []string{"bar"},
			MatchLabels: &core.LabelSelector{
				MatchExpressions: []*core.LabelSelectorRequirement{
					{Key: "foo", Operator: string(core.LabelSelectorOpExists)},
				},
			},
		}, nil),
	)
	DescribeTable("RoleBinding", validateEntry[*core.RoleBinding],
		Entry(nil, &core.RoleBinding{}, validation.ErrMissingRequiredField),
		Entry(nil, &core.RoleBinding{Id: "foo"}, validation.ErrMissingRequiredField),
		Entry(nil, &core.RoleBinding{Id: "\\", RoleId: "foo"}, validation.ErrInvalidID),
		Entry(nil, &core.RoleBinding{Id: "foo", RoleId: "\\"}, validation.ErrInvalidID),
		Entry(nil, &core.RoleBinding{
			Id:       "foo",
			RoleId:   "bar",
			Subjects: []string{"\\"},
		}, validation.ErrInvalidSubjectName),
		Entry(nil, &core.RoleBinding{
			Id:       "foo",
			RoleId:   "bar",
			Subjects: []string{"bar"},
		}, nil),
	)
	DescribeTable("Reference", validateEntry[*core.Reference],
		Entry(nil, &core.Reference{}, validation.ErrMissingRequiredField),
		Entry(nil, &core.Reference{Id: "\\"}, validation.ErrInvalidID),
		Entry(nil, &core.Reference{Id: "foo"}, nil),
	)
	DescribeTable("SubjectAccessRequest", validateEntry[*core.SubjectAccessRequest],
		Entry(nil, &core.SubjectAccessRequest{}, validation.ErrMissingRequiredField),
		Entry(nil, &core.SubjectAccessRequest{Subject: "\\"}, validation.ErrInvalidSubjectName),
		Entry(nil, &core.SubjectAccessRequest{Subject: "foo"}, nil),
	)
	DescribeTable("MatchOptions", validateEntry[core.MatchOptions],
		Entry(nil, core.MatchOptions_Default, nil),
		Entry(nil, core.MatchOptions_EmptySelectorMatchesNone, nil),
		Entry(nil, core.MatchOptions(100), validation.ErrInvalidValue),
		Entry(nil, core.MatchOptions(-1), validation.ErrInvalidValue),
	)
	DescribeTable("TokenCapability", validateEntry[*core.TokenCapability],
		Entry(nil, &core.TokenCapability{}, validation.ErrMissingRequiredField),
		Entry(nil, &core.TokenCapability{Type: "foo"}, nil),
		Entry(nil, &core.TokenCapability{
			Type:      "foo",
			Reference: &core.Reference{Id: "\\"},
		}, validation.ErrInvalidID),
		Entry(nil, &core.TokenCapability{
			Type:      "foo",
			Reference: &core.Reference{Id: "foo"},
		}, nil),
	)
})
