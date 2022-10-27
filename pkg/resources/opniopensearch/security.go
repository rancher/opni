package opniopensearch

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/rancher/opni/pkg/util"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	bcryptCost       = 5
	internalUsersKey = "internal_users.yml"
)

var (
	internalUsersTemplate = template.Must(template.New("natsconn").Parse(`_meta:
  type: "internalusers"
  config_version: 2

# Define your internal users here

## Demo users

admin:
  hash: "$2a$12$VcCDgh2NDk07JGN0rjGbM.Ad41qVR/YFJcgHp0UGns5JDymv..TOG"
  reserved: true
  backend_roles:
  - "admin"
  description: "Demo admin user"

internalopni:
  hash: "{{ . }}"
  reserved: true
  backend_roles:
  - "admin"
  description: "Internal admin user"

kibanaserver:
  hash: "$2a$12$4AcgAt3xwOWadA5s5blL6ev39OXDNhmOesEoo33eZtrq2N0YrU3H."
  reserved: true
  description: "Demo OpenSearch Dashboards user"

readall:
  hash: "$2a$12$ae4ycwzwvLtZxwZ82RmiEunBbIPiAmGZduBAjKN0TXdwQFtCwARz2"
  reserved: false
  backend_roles:
  - "readall"
  description: "Demo readall user"

snapshotrestore:
  hash: "$2y$12$DpwmetHKwgYnorbgdvORCenv4NAK8cPUg8AI6pxLCuWf/ALc0.v7W"
  reserved: false
  backend_roles:
  - "snapshotrestore"
  description: "Demo snapshotrestore user"`))
)

func (r *Reconciler) generatePasswordObjects() (retObjects []runtime.Object, retErr error) {
	password := util.GenerateRandomString(16)
	securityconfig, retErr := r.generateInternalUsers(password)
	if retErr != nil {
		return
	}
	retObjects = append(retObjects, securityconfig)
	retObjects = append(retObjects, r.generateAuthSecret(password))

	return
}

func (r *Reconciler) generateInternalUsers(password []byte) (runtime.Object, error) {
	lg := log.FromContext(r.ctx)
	lg.Info("generating bcrypt hash, this is slow")
	hash, err := bcrypt.GenerateFromPassword(password, bcryptCost)
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	err = internalUsersTemplate.Execute(&buffer, string(hash))
	if err != nil {
		return nil, err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-securityconfig", r.instance.Name),
			Namespace: r.instance.Namespace,
		},
		Data: map[string][]byte{
			internalUsersKey: buffer.Bytes(),
		},
	}
	ctrl.SetControllerReference(r.instance, secret, r.client.Scheme())
	return secret, nil
}

func (r *Reconciler) generateAuthSecret(password []byte) runtime.Object {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-internal-auth", r.instance.Name),
			Namespace: r.instance.Namespace,
		},
		Data: map[string][]byte{
			"username": []byte("internalopni"),
			"password": password,
		},
	}
	ctrl.SetControllerReference(r.instance, secret, r.client.Scheme())
	return secret
}
