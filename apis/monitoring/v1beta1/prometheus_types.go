package v1beta1

import (
	promoperatorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/rancher/wrangler/pkg/crd"
	"github.com/rancher/wrangler/pkg/schemas/openapi"
)

func ServiceMonitorCRD() (*crd.CRD, error) {
	schema, err := openapi.ToOpenAPIFromStruct(promoperatorv1.ServiceMonitor{})
	if err != nil {
		return nil, err
	}
	return &crd.CRD{
		GVK:        promoperatorv1.SchemeGroupVersion.WithKind("ServiceMonitor"),
		PluralName: "servicemonitors",
		Status:     true,
		Schema:     schema,
	}, nil
}

func PodMonitorCRD() (*crd.CRD, error) {
	schema, err := openapi.ToOpenAPIFromStruct(promoperatorv1.PodMonitor{})
	if err != nil {
		return nil, err
	}
	return &crd.CRD{
		GVK:        promoperatorv1.SchemeGroupVersion.WithKind("PodMonitor"),
		PluralName: "podmonitors",
		Status:     true,
		Schema:     schema,
	}, nil
}

func PrometheusCRD() (*crd.CRD, error) {
	schema, err := openapi.ToOpenAPIFromStruct(promoperatorv1.Prometheus{})
	if err != nil {
		return nil, err
	}
	return &crd.CRD{
		GVK:        promoperatorv1.SchemeGroupVersion.WithKind("Prometheus"),
		PluralName: "prometheuses",
		Status:     true,
		Schema:     schema,
	}, nil
}
