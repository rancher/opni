package elastic

import (
	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	defaultLoggingConfig = `appender:
  console:
    layout:
      conversionPattern: '[%d{ISO8601}][%-5p][%-25c] %m%n'
      type: consolePattern
    type: console
es.logger.level: INFO
logger:
  action: DEBUG
  com.amazonaws: WARN
rootLogger: ${es.logger.level}, console`
)

func (r *Reconciler) elasticConfigSecret() resources.Resource {
	secretName := "opni-es-config"
	if r.opniCluster.Spec.Elastic.ConfigSecret != nil {
		// If auth secret is provided, use it instead of creating a new one. If
		// the secret doesn't exist, create one with the given name.
		secretName = r.opniCluster.Spec.Elastic.ConfigSecret.Name
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: r.opniCluster.Namespace,
		},
		StringData: map[string]string{
			"logging.yml": defaultLoggingConfig,
		},
	}

	return resources.Present(secret)
}
