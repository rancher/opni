package elastic

import (
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
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
		Name:       "kibana",
		Port:       5601,
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
	labels := resources.NewOpensearchLabels()

	// Create data service
	dataSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-es-data",
			Namespace: r.opniCluster.Namespace,
			Labels:    labels.WithRole(v1beta2.OpensearchDataRole),
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
				"role": string(v1beta2.OpensearchDataRole),
			},
		},
	}
	clientSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-es-client",
			Namespace: r.opniCluster.Namespace,
			Labels:    labels.WithRole(v1beta2.OpensearchClientRole),
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
				"role": string(v1beta2.OpensearchClientRole),
			},
		},
	}
	discoverySvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-es-discovery",
			Namespace: r.opniCluster.Namespace,
			Labels:    labels.WithRole(v1beta2.OpensearchMasterRole),
		},
		Spec: corev1.ServiceSpec{
			PublishNotReadyAddresses: true,
			Ports: []corev1.ServicePort{
				transportPort,
			},
			ClusterIP: corev1.ClusterIPNone,
			Selector: map[string]string{
				"role": string(v1beta2.OpensearchMasterRole),
			},
		},
	}
	kibanaSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-es-kibana",
			Namespace: r.opniCluster.Namespace,
			Labels:    labels.WithRole(v1beta2.OpensearchDashboardsRole),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				kibanaPort,
			},
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"role": string(v1beta2.OpensearchDashboardsRole),
			},
		},
	}

	ctrl.SetControllerReference(r.opniCluster, dataSvc, r.client.Scheme())
	ctrl.SetControllerReference(r.opniCluster, clientSvc, r.client.Scheme())
	ctrl.SetControllerReference(r.opniCluster, discoverySvc, r.client.Scheme())
	ctrl.SetControllerReference(r.opniCluster, kibanaSvc, r.client.Scheme())

	return []resources.Resource{
		resources.Present(dataSvc),
		resources.Present(clientSvc),
		resources.Present(discoverySvc),
		resources.Present(kibanaSvc),
	}
}
