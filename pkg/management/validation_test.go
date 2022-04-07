package management_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/validation"
)

func validateEntry[T validation.Validator](in T, expected error) {
	if expected == nil {
		Expect(in.Validate()).To(Succeed())
	} else {
		Expect(in.Validate()).To(MatchError(expected))
	}
}

var _ = Describe("Validation", func() {
	DescribeTable("CreateBootstrapTokenRequest",
		validateEntry[*management.CreateBootstrapTokenRequest],
		Entry(nil, &management.CreateBootstrapTokenRequest{}, validation.ErrInvalidValue),
		Entry(nil, &management.CreateBootstrapTokenRequest{Ttl: durationpb.New(0)}, validation.ErrInvalidValue),
		Entry(nil, &management.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(1),
			Labels: map[string]string{
				"\\": "bar",
			},
		}, validation.ErrInvalidLabelName),
		Entry(nil, &management.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(1),
			Labels: map[string]string{
				"foo": "bar",
			},
			Capabilities: []*core.TokenCapability{
				{
					Type: "foo",
					Reference: &core.Reference{
						Id: "\\",
					},
				},
			},
		}, validation.ErrInvalidID),
	)
	DescribeTable("ListClustersRequest",
		validateEntry[*management.ListClustersRequest],
		Entry(nil, &management.ListClustersRequest{}, nil),
		Entry(nil, &management.ListClustersRequest{
			MatchLabels: &core.LabelSelector{
				MatchLabels: map[string]string{
					"\\": "bar",
				},
			},
		}, validation.ErrInvalidLabelName),
		Entry(nil, &management.ListClustersRequest{
			MatchLabels: &core.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},
			MatchOptions: 1234,
		}, validation.ErrInvalidValue),
		Entry(nil, &management.ListClustersRequest{
			MatchLabels: &core.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},
			MatchOptions: core.MatchOptions_EmptySelectorMatchesNone,
		}, nil),
	)
	DescribeTable("EditClusterRequest",
		validateEntry[*management.EditClusterRequest],
		Entry(nil, &management.EditClusterRequest{}, validation.ErrMissingRequiredField),
		Entry(nil, &management.EditClusterRequest{
			Cluster: &core.Reference{
				Id: "\\",
			},
		}, validation.ErrInvalidID),
		Entry(nil, &management.EditClusterRequest{
			Cluster: &core.Reference{
				Id: "foo",
			},
			Labels: map[string]string{
				"\\": "bar",
			},
		}, validation.ErrInvalidLabelName),
		Entry(nil, &management.EditClusterRequest{
			Cluster: &core.Reference{
				Id: "foo",
			},
			Labels: map[string]string{
				"foo": "bar",
			},
		}, nil),
	)
	DescribeTable("WatchClustersRequest",
		validateEntry[*management.WatchClustersRequest],
		Entry(nil, &management.WatchClustersRequest{}, nil),
		Entry(nil, &management.WatchClustersRequest{
			KnownClusters: &core.ReferenceList{
				Items: []*core.Reference{
					{
						Id: "\\",
					},
				},
			},
		}, validation.ErrInvalidID),
		Entry(nil, &management.WatchClustersRequest{
			KnownClusters: &core.ReferenceList{
				Items: []*core.Reference{
					{
						Id: "foo",
					},
				},
			},
		}, nil),
	)
	DescribeTable("UpdateConfigRequest",
		validateEntry[*management.UpdateConfigRequest],
		Entry(nil, &management.UpdateConfigRequest{}, validation.ErrMissingRequiredField),
		Entry(nil, &management.UpdateConfigRequest{
			Documents: []*management.ConfigDocument{
				{
					Json: []byte("{"),
				},
			},
		}, validation.ErrInvalidValue),
		Entry(nil, &management.UpdateConfigRequest{
			Documents: []*management.ConfigDocument{
				{
					Json: []byte("{}"),
				},
			},
		}, nil),
	)
})
