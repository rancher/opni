package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/validation"
)

func validateEntry[T validation.Validator](in T, expected error) {
	if expected == nil {
		Expect(in.Validate()).To(Succeed())
	} else {
		Expect(in.Validate()).To(MatchError(expected))
	}
}

var _ = Describe("Validation", Label("unit"), func() {
	DescribeTable("Cluster", validateEntry[*v1.Cluster],
		Entry(nil, &v1.Cluster{Id: "foo"}, nil),
		Entry(nil, &v1.Cluster{}, validation.ErrMissingRequiredField),
		Entry(nil, &v1.Cluster{Id: "$"}, validation.ErrInvalidID),
		Entry(nil, &v1.Cluster{
			Id: "foo",
			Metadata: &v1.ClusterMetadata{
				Labels: map[string]string{"\\": "foo"},
			},
		}, validation.ErrInvalidLabelName),
	)
	DescribeTable("LabelSelector", validateEntry[*v1.LabelSelector],
		Entry(nil, &v1.LabelSelector{}, nil),
		Entry(nil, &v1.LabelSelector{
			MatchLabels: map[string]string{"foo": "\\"},
		}, validation.ErrInvalidLabelValue),
		Entry(nil, &v1.LabelSelector{
			MatchLabels: map[string]string{"foo": "bar"},
			MatchExpressions: []*v1.LabelSelectorRequirement{
				{Key: "foo", Operator: "invalid"},
			},
		}, validation.ErrInvalidValue),
	)
	DescribeTable("LabelSelectorRequirement", validateEntry[*v1.LabelSelectorRequirement],
		Entry(nil, &v1.LabelSelectorRequirement{}, validation.ErrMissingRequiredField),
		Entry(nil, &v1.LabelSelectorRequirement{Key: "foo"}, validation.ErrMissingRequiredField),
		Entry(nil, &v1.LabelSelectorRequirement{
			Key:      "\\",
			Operator: string(v1.LabelSelectorOpExists),
		}, validation.ErrInvalidLabelName),
		Entry(nil, &v1.LabelSelectorRequirement{
			Key:      "foo",
			Operator: "invalid",
		}, validation.ErrInvalidValue),
		Entry(nil, &v1.LabelSelectorRequirement{
			Key:      "foo",
			Operator: string(v1.LabelSelectorOpExists),
			Values:   []string{"\\"},
		}, validation.ErrInvalidLabelValue),
		Entry(nil, &v1.LabelSelectorRequirement{
			Key:      "foo",
			Operator: string(v1.LabelSelectorOpExists),
			Values:   []string{"bar"},
		}, nil),
	)
	DescribeTable("Role", validateEntry[*v1.Role],
		Entry(nil, &v1.Role{}, validation.ErrMissingRequiredField),
		Entry(nil, &v1.Role{Id: "\\"}, validation.ErrInvalidID),
		Entry(nil, &v1.Role{
			Id:         "foo",
			ClusterIDs: []string{"\\"},
		}, validation.ErrInvalidID),
		Entry(nil, &v1.Role{
			Id:         "foo",
			ClusterIDs: []string{"bar"},
			MatchLabels: &v1.LabelSelector{
				MatchExpressions: []*v1.LabelSelectorRequirement{
					{Key: "foo", Operator: "invalid"},
				},
			},
		}, validation.ErrInvalidValue),
		Entry(nil, &v1.Role{
			Id:         "foo",
			ClusterIDs: []string{"bar"},
			MatchLabels: &v1.LabelSelector{
				MatchExpressions: []*v1.LabelSelectorRequirement{
					{Key: "foo", Operator: string(v1.LabelSelectorOpExists)},
				},
			},
		}, nil),
	)
	DescribeTable("RoleBinding", validateEntry[*v1.RoleBinding],
		Entry(nil, &v1.RoleBinding{}, validation.ErrMissingRequiredField),
		Entry(nil, &v1.RoleBinding{Id: "foo"}, validation.ErrMissingRequiredField),
		Entry(nil, &v1.RoleBinding{Id: "\\", RoleId: "foo"}, validation.ErrInvalidID),
		Entry(nil, &v1.RoleBinding{Id: "foo", RoleId: "\\"}, validation.ErrInvalidID),
		Entry(nil, &v1.RoleBinding{
			Id:       "foo",
			RoleId:   "bar",
			Subjects: []string{"\\"},
		}, validation.ErrInvalidSubjectName),
		Entry(nil, &v1.RoleBinding{
			Id:       "foo",
			RoleId:   "bar",
			Subjects: []string{"bar"},
		}, nil),
	)
	DescribeTable("Reference", validateEntry[*v1.Reference],
		Entry(nil, &v1.Reference{}, validation.ErrMissingRequiredField),
		Entry(nil, &v1.Reference{Id: "\\"}, validation.ErrInvalidID),
		Entry(nil, &v1.Reference{Id: "foo"}, nil),
	)
	DescribeTable("SubjectAccessRequest", validateEntry[*v1.SubjectAccessRequest],
		Entry(nil, &v1.SubjectAccessRequest{}, validation.ErrMissingRequiredField),
		Entry(nil, &v1.SubjectAccessRequest{Subject: "\\"}, validation.ErrInvalidSubjectName),
		Entry(nil, &v1.SubjectAccessRequest{Subject: "foo"}, nil),
	)
	DescribeTable("MatchOptions", validateEntry[v1.MatchOptions],
		Entry(nil, v1.MatchOptions_Default, nil),
		Entry(nil, v1.MatchOptions_EmptySelectorMatchesNone, nil),
		Entry(nil, v1.MatchOptions(100), validation.ErrInvalidValue),
		Entry(nil, v1.MatchOptions(-1), validation.ErrInvalidValue),
	)
	DescribeTable("TokenCapability", validateEntry[*v1.TokenCapability],
		Entry(nil, &v1.TokenCapability{}, validation.ErrMissingRequiredField),
		Entry(nil, &v1.TokenCapability{Type: "foo"}, nil),
		Entry(nil, &v1.TokenCapability{
			Type:      "foo",
			Reference: &v1.Reference{Id: "\\"},
		}, validation.ErrInvalidID),
		Entry(nil, &v1.TokenCapability{
			Type:      "foo",
			Reference: &v1.Reference{Id: "foo"},
		}, nil),
	)
})
