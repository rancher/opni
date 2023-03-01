package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/validation"
)

func validateEntry(in validation.Validator, expected error) {
	if expected == nil {
		Expect(in.Validate()).To(Succeed())
	} else {
		Expect(in.Validate()).To(MatchError(expected))
	}
}

var _ = Describe("Validation", Label("unit"), func() {
	DescribeTable("Cluster", validateEntry,
		Entry(nil, &corev1.Cluster{Id: "foo"}, nil),
		Entry(nil, &corev1.Cluster{}, validation.ErrMissingRequiredField),
		Entry(nil, &corev1.Cluster{Id: "$"}, validation.ErrInvalidID),
		Entry(nil, &corev1.Cluster{
			Id: "foo",
			Metadata: &corev1.ClusterMetadata{
				Labels: map[string]string{"\\": "foo"},
			},
		}, validation.ErrInvalidLabelName),
	)
	DescribeTable("LabelSelector", validateEntry,
		Entry(nil, &corev1.LabelSelector{}, nil),
		Entry(nil, &corev1.LabelSelector{
			MatchLabels: map[string]string{"foo": "\\"},
		}, validation.ErrInvalidLabelValue),
		Entry(nil, &corev1.LabelSelector{
			MatchLabels: map[string]string{"foo": "bar"},
			MatchExpressions: []*corev1.LabelSelectorRequirement{
				{Key: "foo", Operator: "invalid"},
			},
		}, validation.ErrInvalidValue),
		Entry(nil, (&corev1.LabelSelector{
			MatchLabels: map[string]string{"opni.io/name": "bar"},
		}).WithRestrictInternalLabels(), corev1.ErrInternalLabelInSelector),
		Entry(nil, (&corev1.LabelSelector{
			MatchExpressions: []*corev1.LabelSelectorRequirement{
				{Key: "opni.io/name", Operator: string(corev1.LabelSelectorOpExists)},
			},
		}).WithRestrictInternalLabels(), corev1.ErrInternalLabelInSelector),
	)
	DescribeTable("LabelSelectorRequirement", validateEntry,
		Entry(nil, &corev1.LabelSelectorRequirement{}, validation.ErrMissingRequiredField),
		Entry(nil, &corev1.LabelSelectorRequirement{Key: "foo"}, validation.ErrMissingRequiredField),
		Entry(nil, &corev1.LabelSelectorRequirement{
			Key:      "\\",
			Operator: string(corev1.LabelSelectorOpExists),
		}, validation.ErrInvalidLabelName),
		Entry(nil, &corev1.LabelSelectorRequirement{
			Key:      "foo",
			Operator: "invalid",
		}, validation.ErrInvalidValue),
		Entry(nil, &corev1.LabelSelectorRequirement{
			Key:      "foo",
			Operator: string(corev1.LabelSelectorOpExists),
			Values:   []string{"\\"},
		}, validation.ErrInvalidLabelValue),
		Entry(nil, &corev1.LabelSelectorRequirement{
			Key:      "foo",
			Operator: string(corev1.LabelSelectorOpExists),
			Values:   []string{"bar"},
		}, nil),
	)
	DescribeTable("Role", validateEntry,
		Entry(nil, &corev1.Role{}, validation.ErrMissingRequiredField),
		Entry(nil, &corev1.Role{Id: "\\"}, validation.ErrInvalidID),
		Entry(nil, &corev1.Role{
			Id:         "foo",
			ClusterIDs: []string{"\\"},
		}, validation.ErrInvalidID),
		Entry(nil, &corev1.Role{
			Id:         "foo",
			ClusterIDs: []string{"bar"},
			MatchLabels: &corev1.LabelSelector{
				MatchExpressions: []*corev1.LabelSelectorRequirement{
					{Key: "foo", Operator: "invalid"},
				},
			},
		}, validation.ErrInvalidValue),
		Entry(nil, &corev1.Role{
			Id:         "foo",
			ClusterIDs: []string{"bar"},
			MatchLabels: &corev1.LabelSelector{
				MatchExpressions: []*corev1.LabelSelectorRequirement{
					{Key: "foo", Operator: string(corev1.LabelSelectorOpExists)},
				},
			},
		}, nil),
	)
	DescribeTable("RoleBinding", validateEntry,
		Entry(nil, &corev1.RoleBinding{}, validation.ErrMissingRequiredField),
		Entry(nil, &corev1.RoleBinding{Id: "foo"}, validation.ErrMissingRequiredField),
		Entry(nil, &corev1.RoleBinding{Id: "\\", RoleId: "foo"}, validation.ErrInvalidID),
		Entry(nil, &corev1.RoleBinding{Id: "foo", RoleId: "\\"}, validation.ErrInvalidID),
		Entry(nil, &corev1.RoleBinding{
			Id:       "foo",
			RoleId:   "bar",
			Subjects: []string{"\\"},
		}, validation.ErrInvalidSubjectName),
		Entry(nil, &corev1.RoleBinding{
			Id:       "foo",
			RoleId:   "bar",
			Subjects: []string{"bar"},
		}, nil),
	)
	DescribeTable("Reference", validateEntry,
		Entry(nil, &corev1.Reference{}, validation.ErrMissingRequiredField),
		Entry(nil, &corev1.Reference{Id: "\\"}, validation.ErrInvalidID),
		Entry(nil, &corev1.Reference{Id: "foo"}, nil),
	)
	DescribeTable("SubjectAccessRequest", validateEntry,
		Entry(nil, &corev1.SubjectAccessRequest{}, validation.ErrMissingRequiredField),
		Entry(nil, &corev1.SubjectAccessRequest{Subject: "\\"}, validation.ErrInvalidSubjectName),
		Entry(nil, &corev1.SubjectAccessRequest{Subject: "foo"}, nil),
	)
	DescribeTable("MatchOptions", validateEntry,
		Entry(nil, corev1.MatchOptions_Default, nil),
		Entry(nil, corev1.MatchOptions_EmptySelectorMatchesNone, nil),
		Entry(nil, corev1.MatchOptions(100), validation.ErrInvalidValue),
		Entry(nil, corev1.MatchOptions(-1), validation.ErrInvalidValue),
	)
	DescribeTable("TokenCapability", validateEntry,
		Entry(nil, &corev1.TokenCapability{}, validation.ErrMissingRequiredField),
		Entry(nil, &corev1.TokenCapability{Type: "foo"}, nil),
		Entry(nil, &corev1.TokenCapability{
			Type:      "foo",
			Reference: &corev1.Reference{Id: "\\"},
		}, validation.ErrInvalidID),
		Entry(nil, &corev1.TokenCapability{
			Type:      "foo",
			Reference: &corev1.Reference{Id: "foo"},
		}, nil),
	)
})
