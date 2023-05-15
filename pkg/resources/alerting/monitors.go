package alerting

import (
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *Reconciler) serviceMonitor(
	name string,
	matchLabels map[string]string,
	endpoints []monitoringv1.Endpoint,
) *monitoringv1.ServiceMonitor {
	svcMonitor := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.gw.Namespace,
			Labels:    matchLabels,
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
			NamespaceSelector: monitoringv1.NamespaceSelector{
				MatchNames: []string{r.gw.Namespace},
			},
			Endpoints: endpoints,
		},
	}
	return svcMonitor
}
