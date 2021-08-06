package elastic

import (
	"fmt"

	"github.com/rancher/opni/pkg/resources"
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
			Name:  "discovery.seed_hosts",
			Value: "opni-es-discovery",
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
	kibanaEnv = []corev1.EnvVar{
		{
			Name:  "CLUSTER_NAME",
			Value: "elasticsearch",
		},
		{
			Name:  "ELASTICSEARCH_HOSTS",
			Value: "https://opni-es-client:9200",
		},
	}
)

func elasticNodeTypeEnv(role resources.ElasticRole) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "node.master",
			Value: fmt.Sprint(role == resources.ElasticMasterRole),
		},
		{
			Name:  "node.ingest",
			Value: fmt.Sprint(role == resources.ElasticClientRole),
		},
		{
			Name:  "node.data",
			Value: fmt.Sprint(role == resources.ElasticDataRole),
		},
	}
}

func (r *Reconciler) javaOptsEnv(role resources.ElasticRole) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name: "ES_JAVA_OPTS",
			Value: javaOpts(func() *corev1.ResourceRequirements {
				switch role {
				case resources.ElasticDataRole:
					if res := r.opniCluster.Spec.Elastic.Workloads.Data.Resources; res != nil {
						return res
					}
				case resources.ElasticClientRole:
					if res := r.opniCluster.Spec.Elastic.Workloads.Client.Resources; res != nil {
						return res
					}
				case resources.ElasticMasterRole:
					if res := r.opniCluster.Spec.Elastic.Workloads.Master.Resources; res != nil {
						return res
					}
				case resources.ElasticKibanaRole:
					if res := r.opniCluster.Spec.Elastic.Workloads.Kibana.Resources; res != nil {
						return res
					}
				}
				return &corev1.ResourceRequirements{}
			}()),
		},
	}
}

func javaOpts(req *corev1.ResourceRequirements) string {
	if memLimit, ok := req.Limits[corev1.ResourceMemory]; ok {
		return fmt.Sprintf("-Xms%[1]dm -Xmx%[1]dm", memLimit.ScaledValue(resource.Mega)/2)
	}
	if memReq, ok := req.Requests[corev1.ResourceMemory]; ok {
		return fmt.Sprintf("-Xms%[1]dm -Xmx%[1]dm", memReq.ScaledValue(resource.Mega)/2)
	}
	return "-Xms512m -Xmx512m"
}

func combineEnvVars(envVars ...[]corev1.EnvVar) (result []corev1.EnvVar) {
	for _, envVars := range envVars {
		result = append(result, envVars...)
	}
	return
}
