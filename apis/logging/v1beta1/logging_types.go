package v1beta1

import (
	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/rancher/wrangler/pkg/crd"
	"github.com/rancher/wrangler/pkg/schemas/openapi"
)

func ClusterOutputCRD() (*crd.CRD, error) {
	schema, err := openapi.ToOpenAPIFromStruct(loggingv1beta1.ClusterOutput{})
	if err != nil {
		return nil, err
	}
	return &crd.CRD{
		GVK:        GroupVersion.WithKind("ClusterOutput"),
		PluralName: "clusteroutputs",
		Status:     true,
		Schema:     schema,
	}, nil
}

func ClusterFlowCRD() (*crd.CRD, error) {
	schema, err := openapi.ToOpenAPIFromStruct(loggingv1beta1.ClusterFlow{})
	if err != nil {
		return nil, err
	}
	return &crd.CRD{
		GVK:        GroupVersion.WithKind("ClusterFlow"),
		PluralName: "clusterflows",
		Status:     true,
		Schema:     schema,
	}, nil
}

func init() {
	SchemeBuilder.Register(
		&loggingv1beta1.Logging{}, &loggingv1beta1.LoggingList{},
		&loggingv1beta1.Flow{}, &loggingv1beta1.FlowList{},
		&loggingv1beta1.Output{}, &loggingv1beta1.OutputList{},
		&loggingv1beta1.ClusterFlow{}, &loggingv1beta1.ClusterFlowList{},
		&loggingv1beta1.ClusterOutput{}, &loggingv1beta1.ClusterOutputList{},
		&loggingv1beta1.SyslogNGClusterFlow{}, &loggingv1beta1.SyslogNGClusterFlowList{},
		&loggingv1beta1.SyslogNGClusterOutput{}, &loggingv1beta1.SyslogNGClusterOutputList{},
		&loggingv1beta1.SyslogNGFlow{}, &loggingv1beta1.SyslogNGFlowList{},
		&loggingv1beta1.SyslogNGOutput{}, &loggingv1beta1.SyslogNGOutputList{},
	)
}
