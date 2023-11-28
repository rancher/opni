package gateway

import (
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	apicorev1 "github.com/rancher/opni/apis/core/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage/crds"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	v1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	promcommon "github.com/prometheus/common/config"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	configv1 "github.com/rancher/opni/pkg/config/v1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/flagutil"
	"github.com/rancher/opni/pkg/util/k8sutil"
	natsutil "github.com/rancher/opni/pkg/util/nats"
	"github.com/samber/lo"
	yamlv3 "gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/structured-merge-diff/v4/fieldpath"
)

func (r *Reconciler) checkCertificateReady(cert *cmv1.Certificate) k8sutil.RequeueOp {
	lg := r.lg.With("name", cert.Name)
	for _, cond := range cert.Status.Conditions {
		if cond.Type == cmv1.CertificateConditionReady {
			if cond.Status == v1.ConditionTrue {
				return k8sutil.DoNotRequeue()
			}
			lg.Info("certificate not ready", "message", cond.Message)
			break
		}
	}
	lg.Info("certificate not ready (no status reported yet)")
	return k8sutil.RequeueAfter(5 * time.Second)
}

func (r *Reconciler) checkConfiguration() k8sutil.RequeueOp {
	gatewayCa := r.gatewayCA().(*cmv1.Certificate)
	gatewayServingCert := r.gatewayServingCert().(*cmv1.Certificate)
	if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(gatewayCa), gatewayCa); err != nil {
		return k8sutil.RequeueAfter(1 * time.Second)
	}
	if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(gatewayServingCert), gatewayServingCert); err != nil {
		return k8sutil.RequeueAfter(1 * time.Second)
	}
	if op := r.checkCertificateReady(gatewayCa); op.ShouldRequeue() {
		return op
	}
	if op := r.checkCertificateReady(gatewayServingCert); op.ShouldRequeue() {
		return op
	}

	caCert := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gatewayCa.Spec.SecretName,
			Namespace: r.gw.Namespace,
		},
	}
	servingCert := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gatewayServingCert.Spec.SecretName,
			Namespace: r.gw.Namespace,
		},
	}
	if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(caCert), caCert); err != nil {
		return k8sutil.RequeueErr(err)
	}
	if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(servingCert), servingCert); err != nil {
		return k8sutil.RequeueErr(err)
	}

	clear(caCert.Data["ca.key"])
	clear(servingCert.Data["tls.key"])

	certs := &configv1.CertsSpec{
		CaCertData:      lo.ToPtr(string(caCert.Data["ca.crt"])),
		ServingCertData: lo.ToPtr(string(servingCert.Data["tls.crt"])),
		ServingKey:      lo.ToPtr("/run/opni/certs/tls.key"),
	}

	if shouldInitializeConfig(r.gw) {
		return r.initDefaultActiveConfig(certs)
	}

	// if certificates have changed, update the config and requeue
	if r.gw.Spec.Config.Certs.GetCaCertData() != certs.GetCaCertData() ||
		r.gw.Spec.Config.Certs.GetServingCertData() != certs.GetServingCertData() {
		r.lg.Info("gateway certificates have changed, updating config")
		r.gw.Spec.Config.Certs = certs
		err := r.client.Patch(r.ctx, r.gw, client.Apply, client.ForceOwnership, client.FieldOwner(crds.FieldManagerName))

		if err != nil {
			return k8sutil.RequeueErr(err)
		}
		return k8sutil.Requeue()
	}
	return k8sutil.DoNotRequeue()
}

func shouldInitializeConfig(gw *apicorev1.Gateway) bool {
	if gw.Spec.Config == nil {
		return true
	}
	return k8sutil.FieldManagerForPath(gw, fieldpath.MakePathOrDie("spec", "config")) != crds.FieldManagerName
}

func (r *Reconciler) initDefaultActiveConfig(certs *configv1.CertsSpec) k8sutil.RequeueOp {
	conf := &configv1.GatewayConfigSpec{}
	flagutil.LoadDefaults(conf)

	// ensure fields which do not have flags in GatewayConfigSpec are initialized
	conf.Storage = &configv1.StorageSpec{
		Backend: r.gw.Spec.Config.GetStorage().GetBackend().Enum(),
	}
	conf.Auth = &configv1.AuthSpec{}

	conf.Server.AdvertiseAddress = lo.ToPtr("${POD_IP}:9090")
	conf.Management.AdvertiseAddress = lo.ToPtr("${POD_IP}:11090")
	conf.Relay.AdvertiseAddress = lo.ToPtr("${POD_IP}:11190")
	conf.Dashboard.AdvertiseAddress = lo.ToPtr("${POD_IP}:12080")
	conf.Keyring.RuntimeKeyDirs = []string{"/run/opni/keyring"}
	conf.Plugins.Dir = lo.ToPtr("/var/lib/opni/plugins")
	conf.Certs = certs

	// config.storage.backend can be set before initial setup, otherwise
	// GetStorage().GetBackend() will return the default enum value
	switch conf.Storage.GetBackend() {
	case configv1.StorageBackend_Etcd:
		conf.Storage.Etcd = &configv1.EtcdSpec{
			Endpoints: []string{"etcd:2379"},
			Certs: &configv1.MTLSSpec{
				ServerCA:   lo.ToPtr("/run/etcd/certs/server/ca.crt"),
				ClientCA:   lo.ToPtr("/run/etcd/certs/client/ca.crt"),
				ClientCert: lo.ToPtr("/run/etcd/certs/client/tls.crt"),
				ClientKey:  lo.ToPtr("/run/etcd/certs/client/tls.key"),
			},
		}
	case configv1.StorageBackend_JetStream:
		nats := &opnicorev1beta1.NatsCluster{}
		if err := r.client.Get(r.ctx, types.NamespacedName{
			Namespace: r.gw.Namespace,
			Name:      r.gw.Spec.NatsRef.Name,
		}, nats); err != nil {
			return k8sutil.RequeueErr(fmt.Errorf("failed to look up nats cluster, cannot configure jetstream storage: %w", err))
		}
		conf.Storage.JetStream = &configv1.JetStreamSpec{
			Endpoint:     lo.ToPtr(natsutil.BuildK8sServiceUrl(nats.Name, nats.Namespace)),
			NkeySeedPath: lo.ToPtr(filepath.Join(natsutil.NkeyDir, natsutil.NkeySeedFilename)),
		}
	}

	err := r.client.Patch(r.ctx, &apicorev1.Gateway{
		TypeMeta: r.gw.TypeMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.gw.Name,
			Namespace: r.gw.Namespace,
			UID:       r.gw.UID,
		},
		Spec: apicorev1.GatewaySpec{
			Config: conf,
		},
	}, client.Apply, client.ForceOwnership, client.FieldOwner(crds.FieldManagerName))
	if err != nil {
		return k8sutil.RequeueErr(err)
	}
	return k8sutil.Requeue()
}

func (r *Reconciler) amtoolConfigMap() resources.Resource {
	mgmtHost := "127.0.0.1:8080"
	alertmanagerURL := url.URL{
		Scheme: "https",
		Host:   mgmtHost,
		Path:   "/plugin_alerting/alertmanager",
	}

	amToolConfig := map[string]string{
		"alertmanager.url": alertmanagerURL.String(),
		"http.config.file": "/etc/amtool/http.yml",
	}

	amToolConfigBytes, err := yamlv3.Marshal(amToolConfig)
	if err != nil {
		r.lg.Error("failed to marshal amtool config", logger.Err(err))
		amToolConfigBytes = []byte{}
	}

	httpConfig := promcommon.HTTPClientConfig{
		TLSConfig: promcommon.TLSConfig{
			CAFile:   "/run/opni/certs/ca.crt",
			CertFile: "/run/opni/certs/tls.crt",
			KeyFile:  "/run/opni/certs/tls.key",
		},
		FollowRedirects: true,
	}
	httpBytes, err := yamlv3.Marshal(httpConfig)
	if err != nil {
		r.lg.Error("failed to marshal http config", logger.Err(err))
		httpBytes = []byte{}
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "amtool-config",
			Namespace: r.gw.Namespace,
		},
		Data: map[string]string{
			"config.yml": string(amToolConfigBytes),
			"http.yml":   string(httpBytes),
		},
	}
	return resources.Present(cm)
}
