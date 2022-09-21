package elastic

import (
	"bytes"
	"errors"
	"fmt"
	"text/template"

	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/samber/lo"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	passwordSecretName      = "opni-es-password"
	internalUsersSecretName = "opni-es-internalusers"
	internalUsersKey        = "internal_users.yml"
	bcryptCost              = 5
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
	if r.opniCluster.Spec.Opensearch.ConfigSecret != nil {
		// If auth secret is provided, use it instead of creating a new one. If
		// the secret doesn't exist, create one with the given name.
		secretName = r.opniCluster.Spec.Opensearch.ConfigSecret.Name
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

func (r *Reconciler) elasticPasswordResourcces() (err error) {
	var password []byte
	var hash []byte
	var buffer bytes.Buffer
	var passwordSecretRef *corev1.SecretKeySelector
	var ok bool

	lg := log.FromContext(r.ctx)
	generatePassword := r.opniCluster.Status.Auth.GenerateOpensearchHash == nil || *r.opniCluster.Status.Auth.GenerateOpensearchHash

	// Fetch or create the password secret
	if r.opniCluster.Spec.Opensearch.AdminPasswordFrom != nil {
		passwordSecretRef = r.opniCluster.Spec.Opensearch.AdminPasswordFrom
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      r.opniCluster.Spec.Opensearch.AdminPasswordFrom.Name,
				Namespace: r.opniCluster.Namespace,
			},
		}
		err = r.client.Get(r.ctx, client.ObjectKeyFromObject(secret), secret)
		if err != nil {
			return
		}
		password, ok = secret.Data[r.opniCluster.Spec.Opensearch.AdminPasswordFrom.Key]
		if !ok {
			return fmt.Errorf("%s key does not exist in %s", r.opniCluster.Spec.Opensearch.AdminPasswordFrom.Key, r.opniCluster.Spec.Opensearch.AdminPasswordFrom.Name)
		}

	} else {
		if generatePassword {
			password = util.GenerateRandomString(8)
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
			err = k8sutil.CreateOrUpdate(r.ctx, r.client, secret)
			if err != nil {
				return err
			}

		} else {
			// fetch the existing secret
			existingSecret := corev1.Secret{}
			err := r.client.Get(r.ctx, types.NamespacedName{
				Name:      passwordSecretName,
				Namespace: r.opniCluster.Namespace,
			}, &existingSecret)

			// If we can't get the secret return an error
			if k8serrors.IsNotFound(err) {
				retry.RetryOnConflict(retry.DefaultRetry, func() error {
					if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniCluster), r.opniCluster); err != nil {
						return err
					}
					r.opniCluster.Status.Auth.GenerateOpensearchHash = lo.ToPtr(true)
					return r.client.Status().Update(r.ctx, r.opniCluster)
				})
				return errors.New("password secret not found, will recreate on next loop")
			} else if err != nil {
				lg.Error(err, "failed to check password secret")
				return err
			}
		}

		passwordSecretRef = &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: passwordSecretName,
			},
			Key: "password",
		}
	}

	// Generate the internal_users secret
	if generatePassword {
		lg.V(1).Info("generating bcrypt hash of password; this is slow")
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      internalUsersSecretName,
				Namespace: r.opniCluster.Namespace,
			},
			Data: map[string][]byte{},
		}

		// Check the namespace for test annotation
		ns := corev1.Namespace{}
		r.client.Get(r.ctx, types.NamespacedName{
			Name: r.opniCluster.Namespace,
		}, &ns)

		if value, ok := ns.Annotations["controller-test"]; ok && value == "true" {
			lg.V(1).Info("test namespace, using minimum bcrypt difficulty to hash password")
			hash, err = bcrypt.GenerateFromPassword(password, 4)
		} else {
			hash, err = bcrypt.GenerateFromPassword(password, bcryptCost)
		}
		if err != nil {
			return
		}

		err = internalUsersTemplate.Execute(&buffer, string(hash))
		if err != nil {
			return
		}

		secret.Data[internalUsersKey] = buffer.Bytes()
		ctrl.SetControllerReference(r.opniCluster, secret, r.client.Scheme())
		err = k8sutil.CreateOrUpdate(r.ctx, r.client, secret)
		if err != nil {
			return
		}
	}

	// Update the status with the password ref
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniCluster), r.opniCluster); err != nil {
			return err
		}
		r.opniCluster.Status.Auth.OpensearchAuthSecretKeyRef = passwordSecretRef
		r.opniCluster.Status.Auth.GenerateOpensearchHash = lo.ToPtr(false)
		return r.client.Status().Update(r.ctx, r.opniCluster)
	})
	return
}
