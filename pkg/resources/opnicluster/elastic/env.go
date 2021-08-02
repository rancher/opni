package elastic

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var (
	elasticContainerEnv = []corev1.EnvVar{
		{
			Name:  "cluster.name",
			Value: "elasticsearch",
		},
		{
			Name:  "network.host",
			Value: "0.0.0.0",
		},
	}
	downwardsAPIEnv = []corev1.EnvVar{
		{
			Name: "node.name",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name: "KUBERNETES_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
		{
			Name: "PROCESSORS",
			ValueFrom: &corev1.EnvVarSource{
				ResourceFieldRef: &corev1.ResourceFieldSelector{
					Resource: "limits.cpu",
				},
			},
		},
	}
)

func elasticNodeTypeEnv(nodeType string) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "node.master",
			Value: fmt.Sprint(nodeType == "master"),
		},
		{
			Name:  "node.ingest",
			Value: fmt.Sprint(nodeType == "client"),
		},
		{
			Name:  "node.data",
			Value: fmt.Sprint(nodeType == "data"),
		},
	}
}

func (r *Reconciler) javaOptsEnv() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "ES_JAVA_OPTS",
			Value: javaOpts(r.opniCluster.Spec.Elastic.Workloads.Data.Resources),
		},
	}
}

func javaOpts(req *corev1.ResourceRequirements) string {
	if cpuLimit, ok := req.Limits[corev1.ResourceCPU]; ok {
		return fmt.Sprintf("-Xms%[1]dm -Xmx%[1]dm", cpuLimit.ScaledValue(resource.Mega))
	}
	if cpuReq, ok := req.Requests[corev1.ResourceCPU]; ok {
		return fmt.Sprintf("-Xms%[1]dm -Xmx%[1]dm", cpuReq.ScaledValue(resource.Mega))
	}
	return "-Xms512m -Xmx512m"
}

func combineEnvVars(envVars ...[]corev1.EnvVar) (result []corev1.EnvVar) {
	for _, envVars := range envVars {
		result = append(result, envVars...)
	}
	return
}
