package elastic

import (
	"fmt"
	"math"
	"strings"

	"github.com/rancher/opni/apis/v1beta2"
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
			Name:  "OPENSEARCH_HOSTS",
			Value: "https://opni-es-client:9200",
		},
	}
)

func (r *Reconciler) elasticNodeTypeEnv(role v1beta2.ElasticRole) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "node.master",
			Value: fmt.Sprint(role == v1beta2.ElasticMasterRole),
		},
		{
			Name:  "node.ingest",
			Value: fmt.Sprint(role == v1beta2.ElasticDataRole),
		},
		{
			Name:  "node.data",
			Value: fmt.Sprint(role == v1beta2.ElasticDataRole),
		},
		{
			Name:  "discovery.seed_hosts",
			Value: "opni-es-discovery",
		},
	}
	if role == v1beta2.ElasticMasterRole && (r.masterSingleton() || !r.opniCluster.Status.OpensearchState.Initialized) {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "cluster.initial_master_nodes",
			Value: "opni-es-master-0",
		})
	}
	return envVars
}

func (r *Reconciler) javaOptsEnv(role v1beta2.ElasticRole) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name: "OPENSEARCH_JAVA_OPTS",
			Value: javaOpts(func() *corev1.ResourceRequirements {
				switch role {
				case v1beta2.ElasticDataRole:
					if res := r.opniCluster.Spec.Elastic.Workloads.Data.Resources; res != nil {
						return res
					}
				case v1beta2.ElasticClientRole:
					if res := r.opniCluster.Spec.Elastic.Workloads.Client.Resources; res != nil {
						return res
					}
				case v1beta2.ElasticMasterRole:
					if res := r.opniCluster.Spec.Elastic.Workloads.Master.Resources; res != nil {
						return res
					}
				case v1beta2.ElasticKibanaRole:
					if res := r.opniCluster.Spec.Elastic.Workloads.Kibana.Resources; res != nil {
						return res
					}
				}
				return &corev1.ResourceRequirements{}
			}()),
		},
	}
}

func (r *Reconciler) zenMastersEnv() []corev1.EnvVar {
	if r.opniCluster.Spec.Elastic.Workloads.Master.Replicas == nil {
		return []corev1.EnvVar{}
	}
	quorum := math.Round(float64(*r.opniCluster.Spec.Elastic.Workloads.Master.Replicas) / 2)
	return []corev1.EnvVar{
		{
			Name:  "discovery.zen.minimum_master_nodes",
			Value: fmt.Sprintf("%.0f", quorum),
		},
	}
}

func (r *Reconciler) esPasswordEnv() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name: "ES_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: r.opniCluster.Status.Auth.ElasticsearchAuthSecretKeyRef,
			},
		},
	}
}

func (r *Reconciler) masterSingleton() bool {
	return (r.opniCluster.Spec.Elastic.Workloads.Master.Replicas == nil ||
		*r.opniCluster.Spec.Elastic.Workloads.Master.Replicas == int32(1)) &&
		(r.opniCluster.Spec.Elastic.Persistence == nil ||
			!r.opniCluster.Spec.Elastic.Persistence.Enabled)
}

func javaOpts(req *corev1.ResourceRequirements) string {
	opts := []string{
		"-Dlog4j2.formatMsgNoLookups=true",
	}
	if memLimit, ok := req.Limits[corev1.ResourceMemory]; ok {
		value := memLimit.ScaledValue(resource.Mega) / 2
		opts = append(opts, fmt.Sprintf("-Xms%dm", value), fmt.Sprintf("-Xmx%dm", value))
	} else if memReq, ok := req.Requests[corev1.ResourceMemory]; ok {
		value := memReq.ScaledValue(resource.Mega) / 2
		opts = append(opts, fmt.Sprintf("-Xms%dm", value), fmt.Sprintf("-Xmx%dm", value))
	} else {
		opts = append(opts, "-Xms512m", "-Xmx512m")
	}

	return strings.Join(opts, " ")
}

func combineEnvVars(envVars ...[]corev1.EnvVar) (result []corev1.EnvVar) {
	for _, envVars := range envVars {
		result = append(result, envVars...)
	}
	return
}
