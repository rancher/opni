package elastic

import (
	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	transportPort = corev1.ServicePort{
		Name: "transport",
		Port: 9300,
	}
	httpPort = corev1.ServicePort{
		Name: "http",
		Port: 9200,
	}
	metricsPort = corev1.ServicePort{
		Name: "metrics",
		Port: 9600,
	}
	rcaPort = corev1.ServicePort{
		Name: "rca",
		Port: 9650,
	}
	kibanaPort = corev1.ServicePort{
		Name:       "kibana-svc",
		Port:       443,
		TargetPort: intstr.FromInt(5601),
	}
)

func containerPort(servicePort corev1.ServicePort) corev1.ContainerPort {
	return corev1.ContainerPort{
		Name:          servicePort.Name,
		ContainerPort: servicePort.Port,
	}
}

func (r *Reconciler) elasticServices() []resources.Resource {
	// Create data, client, discovery, and kibana services
	labels := resources.NewElasticLabels()

	// Create data service
	dataSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opendistro-es-data-svc",
			Namespace: r.opniCluster.Namespace,
			Labels:    labels.WithRole(resources.ElasticDataRole),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				transportPort,
				httpPort,
				metricsPort,
				rcaPort,
			},
			ClusterIP: corev1.ClusterIPNone,
			Selector: map[string]string{
				"role": string(resources.ElasticMasterRole),
			},
		},
	}
	clientSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opendistro-es-client-svc",
			Namespace: r.opniCluster.Namespace,
			Labels:    labels.WithRole(resources.ElasticClientRole),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				transportPort,
				httpPort,
				metricsPort,
				rcaPort,
			},
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"role": string(resources.ElasticMasterRole),
			},
		},
	}
	discoverySvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opendistro-es-discovery-svc",
			Namespace: r.opniCluster.Namespace,
			Labels:    labels.WithRole(resources.ElasticMasterRole),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				transportPort,
			},
			ClusterIP: corev1.ClusterIPNone,
			Selector: map[string]string{
				"role": string(resources.ElasticMasterRole),
			},
		},
	}
	kibanaSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opendistro-es-kibana-svc",
			Namespace: r.opniCluster.Namespace,
			Labels:    labels.WithRole(resources.ElasticKibanaRole),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				kibanaPort,
			},
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"role": string(resources.ElasticKibanaRole),
			},
		},
	}

	return []resources.Resource{
		resources.Present(dataSvc),
		resources.Present(clientSvc),
		resources.Present(discoverySvc),
		resources.Present(kibanaSvc),
	}
}
