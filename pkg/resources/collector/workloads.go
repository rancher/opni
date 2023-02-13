package collector

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	opniloggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	receiversKey = "receivers.yaml"
	mainKey      = "config.yaml"
)

func (r *Reconciler) receiverConfig() (retData []byte, retReceivers []string, retErr error) {
	if r.collector.Spec.LoggingConfig != nil {
		retData = append(retData, []byte(templateLogAgentK8sReceiver)...)
		retReceivers = append(retReceivers, logReceiverK8s)

		receiver, data, err := r.generateDistributionReceiver()
		if err != nil {
			retErr = err
			return
		}
		retData = append(retData, data...)
		if receiver != "" {
			retReceivers = append(retReceivers, receiver)
		}
	}
	return
}

func (r *Reconciler) generateDistributionReceiver() (receiver string, retBytes []byte, retErr error) {
	config := &opniloggingv1beta1.CollectorConfig{}
	retErr = r.client.Get(r.ctx, types.NamespacedName{
		Name:      r.collector.Spec.LoggingConfig.Name,
		Namespace: r.collector.Spec.SystemNamespace,
	}, config)
	if retErr != nil {
		return
	}
	var providerReceiver bytes.Buffer
	switch config.Spec.Provider {
	case opniloggingv1beta1.LogProviderRKE:
		return logReceiverRKE, []byte(templateLogAgentRKE), nil
	case opniloggingv1beta1.LogProviderK3S:
		journaldDir := "/var/log/journald"
		if config.Spec.K3S != nil && config.Spec.K3S.LogPath != "" {
			journaldDir = config.Spec.K3S.LogPath
		}
		retErr = templateLogAgentK3s.Execute(&providerReceiver, journaldDir)
		if retErr != nil {
			return
		}
		return logReceiverK3s, providerReceiver.Bytes(), nil
	case opniloggingv1beta1.LogProviderRKE2:
		journaldDir := "/var/log/journald"
		if config.Spec.RKE2 != nil && config.Spec.RKE2.LogPath != "" {
			journaldDir = config.Spec.RKE2.LogPath
		}
		retErr = templateLogAgentRKE2.Execute(&providerReceiver, journaldDir)
		if retErr != nil {
			return
		}
		return logReceiverRKE2, providerReceiver.Bytes(), nil
	default:
		return
	}
}

func (r *Reconciler) mainConfig(receivers []string) ([]byte, error) {
	config := LoggingConfig{
		Enabled:   r.collector.Spec.LoggingConfig != nil,
		Receivers: receivers,
	}

	var buffer bytes.Buffer
	err := templateMainConfig.Execute(&buffer, config)
	if err != nil {
		return buffer.Bytes(), err
	}

	return buffer.Bytes(), nil
}

func (r *Reconciler) configMap() (resources.Resource, string) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-collector-config"),
			Namespace: r.collector.Spec.SystemNamespace,
		},
		Data: map[string]string{},
	}

	receiverData, receivers, err := r.receiverConfig()
	if err != nil {
		return resources.Error(cm, err), ""
	}
	cm.Data[receiversKey] = string(receiverData)

	mainData, err := r.mainConfig(receivers)
	if err != nil {
		return resources.Error(cm, err), ""
	}
	cm.Data[mainKey] = string(mainData)

	combinedData := append(receiverData, mainData...)
	hash := sha256.New()
	hash.Write(combinedData)
	configHash := hex.EncodeToString(hash.Sum(nil))

	ctrl.SetControllerReference(r.collector, cm, r.client.Scheme())

	return resources.Present(cm), configHash
}
