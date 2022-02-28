package v1beta1

import (
	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
)

func init() {
	SchemeBuilder.Register(
		&loggingv1beta1.Logging{}, &loggingv1beta1.LoggingList{},
		&loggingv1beta1.Flow{}, &loggingv1beta1.FlowList{},
		&loggingv1beta1.Output{}, &loggingv1beta1.OutputList{},
		&loggingv1beta1.ClusterFlow{}, &loggingv1beta1.ClusterFlowList{},
		&loggingv1beta1.ClusterOutput{}, &loggingv1beta1.ClusterOutputList{},
	)
}
