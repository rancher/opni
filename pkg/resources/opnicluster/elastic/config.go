package elastic

import (
	"bytes"
	"text/template"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	passwordSecretName      = "opni-es-password"
	internalUsersSecretName = "opni-es-internalusers"
	internalUsersKey        = "internal_users.yml"
	bcryptCost              = 12
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

	internalUsersTemplate = template.Must(template.New("internalusers").Parse(`
_meta:
  type: "internalusers"
  config_version: 2
admin:
  hash: "{{ . }}"
  reserved: true
  backend_roles:
  - "admin"
  description: "Admin user"
kibanaserver:
  hash: "$2a$12$4AcgAt3xwOWadA5s5blL6ev39OXDNhmOesEoo33eZtrq2N0YrU3H."
  reserved: true
  description: "Kibana server user"
`))
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

	ctrl.SetControllerReference(r.opniCluster, secret, r.client.Scheme())
	return resources.Present(secret)
}

func (r *Reconciler) elasticPasswordResourcces() (resources []resources.Resource, err error) {
	var password []byte
	var buffer bytes.Buffer
	var passwordSecretRef *corev1.SecretKeySelector

	// Fetch or create the password secret
	if r.opniCluster.Spec.Elastic.AdminPasswordFrom != nil {
		passwordSecretRef = r.opniCluster.Spec.Elastic.AdminPasswordFrom
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      r.opniCluster.Spec.Elastic.AdminPasswordFrom.Name,
				Namespace: r.opniCluster.Namespace,
			},
		}
		err = r.client.Get(r.ctx, client.ObjectKeyFromObject(secret), secret)
		if err != nil {
			return
		}
		password = secret.Data[r.opniCluster.Spec.Elastic.AdminPasswordFrom.Key]
	} else {
		// TODO don't regenerate this every reconciliation loop
		passwordSecretRef = &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: passwordSecretName,
			},
			Key: "password",
		}
		password = util.GenerateRandomPassword()
		resources = append(resources, func() (runtime.Object, reconciler.DesiredState, error) {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      passwordSecretName,
					Namespace: r.opniCluster.Namespace,
				},
				Data: map[string][]byte{
					"password": password,
				},
			}
			ctrl.SetControllerReference(r.opniCluster, secret, r.client.Scheme())
			return secret, reconciler.StateCreated, nil
		})
	}

	// Update the status with the password ref
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniCluster), r.opniCluster); err != nil {
			return err
		}
		r.opniCluster.Status.Auth.ElasticsearchAuthSecretKeyRef = passwordSecretRef
		return r.client.Status().Update(r.ctx, r.opniCluster)
	})
	if err != nil {
		return
	}

	// TODO compare the password against a previously generated hash
	// Generate the internal_users secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      internalUsersSecretName,
			Namespace: r.opniCluster.Namespace,
		},
		Data: map[string][]byte{},
	}

	hash, err := bcrypt.GenerateFromPassword(password, bcryptCost)
	if err != nil {
		return
	}

	err = internalUsersTemplate.Execute(&buffer, string(hash))
	if err != nil {
		return
	}

	secret.Data[internalUsersKey] = buffer.Bytes()
	ctrl.SetControllerReference(r.opniCluster, secret, r.client.Scheme())
	return append(resources, func() (runtime.Object, reconciler.DesiredState, error) { return secret, reconciler.StateCreated, nil }), err
}
