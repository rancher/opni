package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/durationpb"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	v1 "github.com/rancher/opni/pkg/apis/management/v1"
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
	DescribeTable("CreateBootstrapTokenRequest",
		validateEntry[*v1.CreateBootstrapTokenRequest],
		Entry(nil, &v1.CreateBootstrapTokenRequest{}, validation.ErrInvalidValue),
		Entry(nil, &v1.CreateBootstrapTokenRequest{Ttl: durationpb.New(0)}, validation.ErrInvalidValue),
		Entry(nil, &v1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(1),
			Labels: map[string]string{
				"\\": "bar",
			},
		}, validation.ErrInvalidLabelName),
		Entry(nil, &v1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(1),
			Labels: map[string]string{
				"foo": "bar",
			},
			Capabilities: []*corev1.TokenCapability{
				{
					Type: "foo",
					Reference: &corev1.Reference{
						Id: "\\",
					},
				},
			},
		}, validation.ErrInvalidID),
	)
	DescribeTable("ListClustersRequest",
		validateEntry[*v1.ListClustersRequest],
		Entry(nil, &v1.ListClustersRequest{}, nil),
		Entry(nil, &v1.ListClustersRequest{
			MatchLabels: &corev1.LabelSelector{
				MatchLabels: map[string]string{
					"\\": "bar",
				},
			},
		}, validation.ErrInvalidLabelName),
		Entry(nil, &v1.ListClustersRequest{
			MatchLabels: &corev1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},
			MatchOptions: 1234,
		}, validation.ErrInvalidValue),
		Entry(nil, &v1.ListClustersRequest{
			MatchLabels: &corev1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},
			MatchOptions: corev1.MatchOptions_EmptySelectorMatchesNone,
		}, nil),
	)
	DescribeTable("EditClusterRequest",
		validateEntry[*v1.EditClusterRequest],
		Entry(nil, &v1.EditClusterRequest{}, validation.ErrMissingRequiredField),
		Entry(nil, &v1.EditClusterRequest{
			Cluster: &corev1.Reference{
				Id: "\\",
			},
		}, validation.ErrInvalidID),
		Entry(nil, &v1.EditClusterRequest{
			Cluster: &corev1.Reference{
				Id: "foo",
			},
			Labels: map[string]string{
				"\\": "bar",
			},
		}, validation.ErrInvalidLabelName),
		Entry(nil, &v1.EditClusterRequest{
			Cluster: &corev1.Reference{
				Id: "foo",
			},
			Labels: map[string]string{
				"foo": "bar",
			},
		}, nil),
	)
	DescribeTable("WatchClustersRequest",
		validateEntry[*v1.WatchClustersRequest],
		Entry(nil, &v1.WatchClustersRequest{}, nil),
		Entry(nil, &v1.WatchClustersRequest{
			KnownClusters: &corev1.ReferenceList{
				Items: []*corev1.Reference{
					{
						Id: "\\",
					},
				},
			},
		}, validation.ErrInvalidID),
		Entry(nil, &v1.WatchClustersRequest{
			KnownClusters: &corev1.ReferenceList{
				Items: []*corev1.Reference{
					{
						Id: "foo",
					},
				},
			},
		}, nil),
	)
	DescribeTable("UpdateConfigRequest",
		validateEntry[*v1.UpdateConfigRequest],
		Entry(nil, &v1.UpdateConfigRequest{}, validation.ErrMissingRequiredField),
		Entry(nil, &v1.UpdateConfigRequest{
			Documents: []*v1.ConfigDocument{
				{
					Json: []byte("{"),
				},
			},
		}, validation.ErrInvalidValue),
		Entry(nil, &v1.UpdateConfigRequest{
			Documents: []*v1.ConfigDocument{
				{
					Json: []byte("{}"),
				},
			},
		}, nil),
	)
})
