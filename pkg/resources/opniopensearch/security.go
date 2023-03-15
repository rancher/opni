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
	bcryptCost        = 12
	internalUsersKey  = "internal_users.yml"
	securityConfigKey = "config.yml"
)

var (
	internalUsersTemplate = template.Must(template.New("natsconn").Parse(`_meta:
  type: "internalusers"
  config_version: 2

# Define your internal users here

{{ .AdminUser }}:
  hash: "{{ .Admin }}"
  reserved: true
  backend_roles:
  - "admin"
  description: "Internal admin user"

kibanaserver:
  hash: "{{ .Dashboards }}"
  reserved: true
  description: "Demo OpenSearch Dashboards user"`))

	securityConfig = `
---
_meta:
  type: "config"
  config_version: 2

config:
  dynamic:
    http:
      anonymous_auth_enabled: false
      xff:
        enabled: false
        internalProxies: '192\.168\.0\.10|192\.168\.0\.11' # regex pattern
        #internalProxies: '.*' # trust all internal proxies, regex pattern
        #remoteIpHeader:  'x-forwarded-for'
        ###### see https://docs.oracle.com/javase/7/docs/api/java/util/regex/Pattern.html for regex help
        ###### more information about XFF https://en.wikipedia.org/wiki/X-Forwarded-For
        ###### and here https://tools.ietf.org/html/rfc7239
        ###### and https://tomcat.apache.org/tomcat-8.0-doc/config/valve.html#Remote_IP_Valve
    authc:
      basic_internal_auth_domain:
        description: "Authenticate via HTTP Basic against internal users database"
        http_enabled: true
        transport_enabled: true
        order: 2
        http_authenticator:
          type: basic
          challenge: true
        authentication_backend:
          type: intern
      clientcert_auth_domain:
        description: "Authenticate via SSL client certificates"
        http_enabled: true
        transport_enabled: false
        order: 1
        http_authenticator:
          type: clientcert
          config:
            username_attribute: cn #optional, if omitted DN becomes username
            challenge: false
        authentication_backend:
          type: noop
`
)

type internalUsersHashes struct {
	AdminUser  string
	Admin      string
	Dashboards string
}

type internalUsersPasswords struct {
	admin      []byte
	dashboards []byte
}

func (r *Reconciler) generatePasswordObjects() (retObjects []runtime.Object, retErr error) {
	adminPassword := util.GenerateRandomString(16)
	dashboardsPassword := util.GenerateRandomString(16)
	securityconfig, retErr := r.generateInternalUsers(internalUsersPasswords{
		admin:      adminPassword,
		dashboards: dashboardsPassword,
	})
	if retErr != nil {
		return
	}
	retObjects = append(retObjects, securityconfig)
	retObjects = append(retObjects, r.generateAuthSecret(adminPassword))
	retObjects = append(retObjects, r.generateDashboardsSecret(dashboardsPassword))

	return
}

func (r *Reconciler) generateInternalUsers(passwords internalUsersPasswords) (runtime.Object, error) {
	lg := log.FromContext(r.ctx)
	lg.Info("generating bcrypt hash, this is slow")
	adminHash, err := bcrypt.GenerateFromPassword(passwords.admin, bcryptCost)
	if err != nil {
		return nil, err
	}
	dashHash, err := bcrypt.GenerateFromPassword(passwords.dashboards, bcryptCost)
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	err = internalUsersTemplate.Execute(&buffer, internalUsersHashes{
		AdminUser:  internalUsername,
		Admin:      string(adminHash),
		Dashboards: string(dashHash),
	})
	if err != nil {
		return nil, err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-securityconfig", r.instance.Name),
			Namespace: r.instance.Namespace,
		},
		Data: map[string][]byte{
			internalUsersKey:  buffer.Bytes(),
			securityConfigKey: []byte(securityConfig),
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
			"username": []byte(internalUsername),
			"password": password,
		},
	}
	ctrl.SetControllerReference(r.instance, secret, r.client.Scheme())
	return secret
}

func (r *Reconciler) generateDashboardsSecret(password []byte) runtime.Object {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-dashboards-auth", r.instance.Name),
			Namespace: r.instance.Namespace,
		},
		Data: map[string][]byte{
			"username": []byte("kibanaserver"),
			"password": password,
		},
	}
	ctrl.SetControllerReference(r.instance, secret, r.client.Scheme())
	return secret
}
