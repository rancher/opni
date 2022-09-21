package gateway

import (
	"fmt"
	"strings"
	"time"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/k8sutil"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	gatewayCASecret      = "opni-gateway-ca-keys"
	gatewayServingSecret = "opni-gateway-serving-cert"
)

func (r *Reconciler) certs() ([]resources.Resource, error) {
	list := []resources.Resource{}
	for _, obj := range []client.Object{
		r.selfsignedIssuer(),
		r.gatewayCA(),
		r.gatewayCAIssuer(),
		r.gatewayServingCert(),
		r.cortexIntermediateCA(),
		r.cortexIntermediateCAIssuer(),
		r.cortexClientCA(),
		r.cortexClientCAIssuer(),
		r.cortexClientCert(),
		r.cortexServingCert(),
		r.etcdIntermediateCA(),
		r.etcdIntermediateCAIssuer(),
		r.etcdClientCert(),
		r.etcdServingCert(),
		r.grafanaCert(),
	} {
		r.setOwner(obj)
		list = append(list, resources.Present(obj))
	}
	return list, nil
}

func (r *Reconciler) selfsignedIssuer() client.Object {
	return &cmv1.ClusterIssuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-gateway-selfsigned-issuer",
			Namespace: r.namespace,
		},
		Spec: cmv1.IssuerSpec{
			IssuerConfig: cmv1.IssuerConfig{
				SelfSigned: &cmv1.SelfSignedIssuer{},
			},
		},
	}
}

func (r *Reconciler) gatewayCA() client.Object {
	return &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-gateway-ca",
			Namespace: r.namespace,
		},
		Spec: cmv1.CertificateSpec{
			IsCA:       true,
			CommonName: "opni-gateway-ca",
			SecretName: gatewayCASecret,
			PrivateKey: &cmv1.CertificatePrivateKey{
				Algorithm: cmv1.Ed25519KeyAlgorithm,
				Encoding:  cmv1.PKCS1,
			},
			IssuerRef: cmmetav1.ObjectReference{
				Group: "cert-manager.io",
				Kind:  "ClusterIssuer",
				Name:  "opni-gateway-selfsigned-issuer",
			},
		},
	}
}

func (r *Reconciler) gatewayCAIssuer() client.Object {
	return &cmv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-gateway-ca-issuer",
			Namespace: r.namespace,
		},
		Spec: cmv1.IssuerSpec{
			IssuerConfig: cmv1.IssuerConfig{
				CA: &cmv1.CAIssuer{
					SecretName: gatewayCASecret,
				},
			},
		},
	}
}

func (r *Reconciler) gatewayServingCert() client.Object {
	return &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-gateway-serving-cert",
			Namespace: r.namespace,
		},
		Spec: cmv1.CertificateSpec{
			SecretName: gatewayServingSecret,
			PrivateKey: &cmv1.CertificatePrivateKey{
				Algorithm: cmv1.Ed25519KeyAlgorithm,
				Encoding:  cmv1.PKCS1,
			},
			IssuerRef: cmmetav1.ObjectReference{
				Group: "cert-manager.io",
				Kind:  "Issuer",
				Name:  "opni-gateway-ca-issuer",
			},
			DNSNames: []string{
				r.spec.Hostname,
				fmt.Sprintf("opni-monitoring.%s.svc", r.namespace),
				fmt.Sprintf("opni-monitoring.%s.svc.cluster.local", r.namespace),
				fmt.Sprintf("opni-monitoring-internal.%s.svc", r.namespace),
				fmt.Sprintf("opni-monitoring-internal.%s.svc.cluster.local", r.namespace),
			},
		},
	}
}
func (r *Reconciler) grafanaCert() client.Object {
	return &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "grafana-datasource-cert",
			Namespace: r.namespace,
		},
		Spec: cmv1.CertificateSpec{
			SecretName: "grafana-datasource-cert",
			PrivateKey: &cmv1.CertificatePrivateKey{
				Algorithm: cmv1.Ed25519KeyAlgorithm,
				Encoding:  cmv1.PKCS1,
			},
			IssuerRef: cmmetav1.ObjectReference{
				Group: "cert-manager.io",
				Kind:  "Issuer",
				Name:  "opni-gateway-ca-issuer",
			},
			DNSNames: []string{
				fmt.Sprintf("grafana.%s.svc", r.namespace),
				fmt.Sprintf("grafana.%s.svc.cluster.local", r.namespace),
			},
		},
	}
}

func (r *Reconciler) cortexIntermediateCA() client.Object {
	return &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex-intermediate-ca",
			Namespace: r.namespace,
		},
		Spec: cmv1.CertificateSpec{
			IsCA:       true,
			SecretName: "cortex-intermediate-ca-keys",
			PrivateKey: &cmv1.CertificatePrivateKey{
				Algorithm: cmv1.Ed25519KeyAlgorithm,
				Encoding:  cmv1.PKCS1,
			},
			DNSNames: []string{
				"cortex-intermediate-ca",
			},
			IssuerRef: cmmetav1.ObjectReference{
				Group: "cert-manager.io",
				Kind:  "Issuer",
				Name:  "opni-gateway-ca-issuer",
			},
		},
	}
}

func (r *Reconciler) cortexIntermediateCAIssuer() client.Object {
	return &cmv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex-intermediate-ca-issuer",
			Namespace: r.namespace,
		},
		Spec: cmv1.IssuerSpec{
			IssuerConfig: cmv1.IssuerConfig{
				CA: &cmv1.CAIssuer{
					SecretName: "cortex-intermediate-ca-keys",
				},
			},
		},
	}
}

func (r *Reconciler) cortexClientCA() client.Object {
	return &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex-client-ca",
			Namespace: r.namespace,
		},
		Spec: cmv1.CertificateSpec{
			IsCA:       true,
			SecretName: "cortex-client-ca-keys",
			PrivateKey: &cmv1.CertificatePrivateKey{
				Algorithm: cmv1.Ed25519KeyAlgorithm,
				Encoding:  cmv1.PKCS1,
			},
			DNSNames: []string{
				"cortex-client-ca",
			},
			IssuerRef: cmmetav1.ObjectReference{
				Group: "cert-manager.io",
				Kind:  "Issuer",
				Name:  "cortex-intermediate-ca-issuer",
			},
			Usages: []cmv1.KeyUsage{
				cmv1.UsageDigitalSignature,
				cmv1.UsageKeyEncipherment,
				cmv1.UsageClientAuth,
			},
		},
	}
}

func (r *Reconciler) cortexClientCAIssuer() client.Object {
	return &cmv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex-client-ca-issuer",
			Namespace: r.namespace,
		},
		Spec: cmv1.IssuerSpec{
			IssuerConfig: cmv1.IssuerConfig{
				CA: &cmv1.CAIssuer{
					SecretName: "cortex-client-ca-keys",
				},
			},
		},
	}
}

func (r *Reconciler) cortexClientCert() client.Object {
	return &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex-client-cert",
			Namespace: r.namespace,
		},
		Spec: cmv1.CertificateSpec{
			SecretName: "cortex-client-cert-keys",
			PrivateKey: &cmv1.CertificatePrivateKey{
				Algorithm: cmv1.Ed25519KeyAlgorithm,
				Encoding:  cmv1.PKCS1,
			},
			IssuerRef: cmmetav1.ObjectReference{
				Group: "cert-manager.io",
				Kind:  "Issuer",
				Name:  "cortex-client-ca-issuer",
			},
			DNSNames: []string{
				"cortex-client",
			},
			Usages: []cmv1.KeyUsage{
				cmv1.UsageDigitalSignature,
				cmv1.UsageKeyEncipherment,
				cmv1.UsageClientAuth,
			},
		},
	}
}

func (r *Reconciler) cortexServingCert() client.Object {
	return &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex-serving-cert",
			Namespace: r.namespace,
		},
		Spec: cmv1.CertificateSpec{
			SecretName: "cortex-serving-cert-keys",
			PrivateKey: &cmv1.CertificatePrivateKey{
				Algorithm: cmv1.Ed25519KeyAlgorithm,
				Encoding:  cmv1.PKCS1,
			},
			IssuerRef: cmmetav1.ObjectReference{
				Group: "cert-manager.io",
				Kind:  "Issuer",
				Name:  "cortex-intermediate-ca-issuer",
			},
			DNSNames: []string{
				"cortex-server",
				"cortex-alertmanager",
				"cortex-alertmanager-headless",
				"cortex-compactor",
				"cortex-distributor",
				"cortex-distributor-headless",
				"cortex-ingester",
				"cortex-ingester-headless",
				"cortex-querier",
				"cortex-query-frontend",
				"cortex-query-frontend-headless",
				"cortex-ruler",
				"cortex-ruler-headless",
				"cortex-store-gateway",
				"cortex-store-gateway-headless",
				"cortex-purger",
			},
		},
	}
}

func (r *Reconciler) etcdIntermediateCA() client.Object {
	return &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd-intermediate-ca",
			Namespace: r.namespace,
		},
		Spec: cmv1.CertificateSpec{
			IsCA:       true,
			SecretName: "etcd-intermediate-ca-keys",
			PrivateKey: &cmv1.CertificatePrivateKey{
				Algorithm: cmv1.Ed25519KeyAlgorithm,
				Encoding:  cmv1.PKCS1,
			},
			DNSNames: []string{
				"etcd-intermediate-ca",
			},
			IssuerRef: cmmetav1.ObjectReference{
				Group: "cert-manager.io",
				Kind:  "Issuer",
				Name:  "opni-gateway-ca-issuer",
			},
			Usages: []cmv1.KeyUsage{
				cmv1.UsageDigitalSignature,
				cmv1.UsageKeyEncipherment,
				cmv1.UsageClientAuth,
				cmv1.UsageServerAuth,
			},
		},
	}
}

func (r *Reconciler) etcdIntermediateCAIssuer() client.Object {
	return &cmv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd-intermediate-ca-issuer",
			Namespace: r.namespace,
		},
		Spec: cmv1.IssuerSpec{
			IssuerConfig: cmv1.IssuerConfig{
				CA: &cmv1.CAIssuer{
					SecretName: "etcd-intermediate-ca-keys",
				},
			},
		},
	}
}

func (r *Reconciler) etcdClientCert() client.Object {
	return &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd-client-cert",
			Namespace: r.namespace,
		},
		Spec: cmv1.CertificateSpec{
			CommonName: "root", // etcd user
			SecretName: "etcd-client-cert-keys",
			PrivateKey: &cmv1.CertificatePrivateKey{
				Algorithm: cmv1.Ed25519KeyAlgorithm,
				Encoding:  cmv1.PKCS1,
			},
			IssuerRef: cmmetav1.ObjectReference{
				Group: "cert-manager.io",
				Kind:  "Issuer",
				Name:  "etcd-intermediate-ca-issuer",
			},
			DNSNames: []string{
				"root",
			},
			Usages: []cmv1.KeyUsage{
				cmv1.UsageDigitalSignature,
				cmv1.UsageKeyEncipherment,
				cmv1.UsageClientAuth,
			},
		},
	}
}

func (r *Reconciler) etcdServingCert() client.Object {
	return &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd-serving-cert",
			Namespace: r.namespace,
		},
		Spec: cmv1.CertificateSpec{
			CommonName: "root", // etcd user
			SecretName: "etcd-serving-cert-keys",
			PrivateKey: &cmv1.CertificatePrivateKey{
				Algorithm: cmv1.Ed25519KeyAlgorithm,
				Encoding:  cmv1.PKCS1,
			},
			IssuerRef: cmmetav1.ObjectReference{
				Group: "cert-manager.io",
				Kind:  "Issuer",
				Name:  "etcd-intermediate-ca-issuer",
			},
			DNSNames: []string{
				"etcd",
				fmt.Sprintf("etcd.%s.svc.cluster.local", r.namespace),
				fmt.Sprintf("etcd.%s.svc", r.namespace),
				fmt.Sprintf("*.etcd-headless.%s.svc.cluster.local", r.namespace),
				fmt.Sprintf("*.etcd-headless.%s.svc", r.namespace),
			},
			Usages: []cmv1.KeyUsage{
				cmv1.UsageDigitalSignature,
				cmv1.UsageKeyEncipherment,
				cmv1.UsageClientAuth,
				cmv1.UsageServerAuth,
			},
		},
	}
}

func (r *Reconciler) gatewayIngressSecret() (client.Object, *k8sutil.RequeueOp) {
	var sb strings.Builder
	ingressCertSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gateway-ingress-cert",
			Namespace: r.namespace,
		},
		Type: corev1.SecretTypeTLS,
	}

	servingCertSecret := &corev1.Secret{}
	if err := r.client.Get(r.ctx, types.NamespacedName{
		Name:      gatewayServingSecret,
		Namespace: r.namespace,
	}, servingCertSecret); err != nil {
		if k8serrors.IsNotFound(err) {
			requeue := k8sutil.RequeueAfter(time.Second * 5)
			return ingressCertSecret, &requeue
		}
		requeue := k8sutil.RequeueErr(err)
		return ingressCertSecret, &requeue
	}

	key, ok := servingCertSecret.Data[corev1.TLSPrivateKeyKey]
	if !ok {
		r.logger.Info("tls key missing from serving secret, requeueing")
		requeue := k8sutil.RequeueAfter(time.Second * 5)
		return ingressCertSecret, &requeue
	}

	cert, ok := servingCertSecret.Data[corev1.TLSCertKey]
	if !ok {
		r.logger.Info("tls cert missing from serving secret, requeueing")
		requeue := k8sutil.RequeueAfter(time.Second * 5)
		return ingressCertSecret, &requeue
	}
	sb.WriteString(string(cert))

	caSecret := &corev1.Secret{}
	if err := r.client.Get(r.ctx, types.NamespacedName{
		Name:      gatewayCASecret,
		Namespace: r.namespace,
	}, caSecret); err != nil {
		if k8serrors.IsNotFound(err) {
			requeue := k8sutil.RequeueAfter(time.Second * 5)
			return ingressCertSecret, &requeue
		}
		requeue := k8sutil.RequeueErr(err)
		return ingressCertSecret, &requeue
	}

	cert, ok = caSecret.Data["ca.crt"]
	if !ok {
		r.logger.Info("ca cert missing from ca secret, requeueing")
		requeue := k8sutil.RequeueAfter(time.Second * 5)
		return ingressCertSecret, &requeue
	}
	sb.WriteString(string(cert))

	cert, ok = caSecret.Data[corev1.TLSCertKey]
	if !ok {
		r.logger.Info("intermediate cert missing from ca secret, requeueing")
		requeue := k8sutil.RequeueAfter(time.Second * 5)
		return ingressCertSecret, &requeue
	}
	sb.WriteString(string(cert))

	ingressCertSecret.StringData = map[string]string{
		corev1.TLSPrivateKeyKey: string(key),
		corev1.TLSCertKey:       sb.String(),
	}

	return ingressCertSecret, nil
}
