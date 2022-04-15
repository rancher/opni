package gateway

import (
	"fmt"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/rancher/opni/pkg/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	} {
		ctrl.SetControllerReference(r.gateway, obj, r.client.Scheme())
		list = append(list, resources.Present(obj))
	}
	return list, nil
}

func (r *Reconciler) selfsignedIssuer() client.Object {
	return &cmv1.ClusterIssuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-gateway-selfsigned-issuer",
			Namespace: r.gateway.Namespace,
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
			Namespace: r.gateway.Namespace,
		},
		Spec: cmv1.CertificateSpec{
			IsCA:       true,
			CommonName: "opni-gateway-ca",
			SecretName: "opni-gateway-ca-keys",
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
			Namespace: r.gateway.Namespace,
		},
		Spec: cmv1.IssuerSpec{
			IssuerConfig: cmv1.IssuerConfig{
				CA: &cmv1.CAIssuer{
					SecretName: "opni-gateway-ca-keys",
				},
			},
		},
	}
}

func (r *Reconciler) gatewayServingCert() client.Object {
	return &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-gateway-serving-cert",
			Namespace: r.gateway.Namespace,
		},
		Spec: cmv1.CertificateSpec{
			SecretName: "opni-gateway-serving-cert",
			PrivateKey: &cmv1.CertificatePrivateKey{
				Algorithm: cmv1.Ed25519KeyAlgorithm,
				Encoding:  cmv1.PKCS1,
			},
			IssuerRef: cmmetav1.ObjectReference{
				Group: "cert-manager.io",
				Kind:  "Issuer",
				Name:  "opni-gateway-ca-issuer",
			},
			DNSNames: append([]string{
				fmt.Sprintf("opni-monitoring.%s.svc", r.gateway.Namespace),
				fmt.Sprintf("opni-monitoring.%s.svc.cluster.local", r.gateway.Namespace),
			}, r.gateway.Spec.Hostname),
		},
	}
}

func (r *Reconciler) cortexIntermediateCA() client.Object {
	return &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex-intermediate-ca",
			Namespace: r.gateway.Namespace,
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
			Namespace: r.gateway.Namespace,
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
			Namespace: r.gateway.Namespace,
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
			Namespace: r.gateway.Namespace,
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
			Namespace: r.gateway.Namespace,
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
			Namespace: r.gateway.Namespace,
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
				"cortex-store-gateway",
				"cortex-store-gateway-headless",
			},
		},
	}
}

func (r *Reconciler) etcdIntermediateCA() client.Object {
	return &cmv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd-intermediate-ca",
			Namespace: r.gateway.Namespace,
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
			Namespace: r.gateway.Namespace,
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
			Namespace: r.gateway.Namespace,
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
			Namespace: r.gateway.Namespace,
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
				"etcd.opni-monitoring.svc.cluster.local",
				"etcd.opni-monitoring.svc",
				"*.etcd-headless.opni-monitoring.svc.cluster.local",
				"*.etcd-headless.opni-monitoring.svc",
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
