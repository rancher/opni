package nats

import (
	"context"
	"fmt"
	"regexp"

	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	DefaultImage               = "bitnami/nats:2.3.2-debian-10-r0"
	DefaultConfigReloaderImage = "connecteverything/nats-server-config-reloader:0.4.5-v1alpha2"
	DefaultPidFilePath         = "/opt/nats/pid"
	DefaultPidFileName         = "nats-server.pid"
	DefaultConfigPath          = "/opt/nats/conf"
	ConfigFileName             = "nats-config.conf"
	DefaultClientPort          = 4222
	DefaultClusterPort         = 6222
	DefaultHTTPPort            = 8222
	NkeyDir                    = "/etc/nkey"
	NkeySeedFilename           = "seed"
)

func BuildK8sServiceUrl(name string, namespace string, port ...int32) string {
	if len(port) == 0 {
		port = append(port, DefaultClientPort)
	}
	return fmt.Sprintf("nats://%s-nats-client.%s.svc:%d", name, namespace, port[0])
}

func ExternalNatsObjects(
	ctx context.Context,
	k8sClient client.Client,
	namespacedNats types.NamespacedName,
) (
	envVars []corev1.EnvVar,
	volumeMounts []corev1.VolumeMount,
	volumes []corev1.Volume,
) {
	lg := log.FromContext(ctx)
	nats := &opnicorev1beta1.NatsCluster{}
	if err := k8sClient.Get(ctx, namespacedNats, nats); err != nil {
		lg.Error("could not fetch nats cluster", logger.Err(err))
		return
	}

	if nats.Status.AuthSecretKeyRef == nil {
		return
	}

	envVars = append(envVars, corev1.EnvVar{
		Name: "NATS_SERVER_URL",
		Value: fmt.Sprintf("nats://%s-nats-client.%s.svc:%d",
			namespacedNats.Name, namespacedNats.Namespace, DefaultClientPort),
	})

	switch nats.Spec.AuthMethod {
	case opnicorev1beta1.NatsAuthPassword:
		var username string
		if nats.Spec.Username == "" {
			username = "nats-user"
		} else {
			username = nats.Spec.Username
		}
		newEnvVars := []corev1.EnvVar{
			{
				Name:  "NATS_USERNAME",
				Value: username,
			},
			{
				Name: "NATS_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: nats.Status.AuthSecretKeyRef,
				},
			},
		}
		envVars = append(envVars, newEnvVars...)
	case opnicorev1beta1.NatsAuthNkey:
		volumes = append(volumes, corev1.Volume{
			Name: "nkey",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: nats.Status.AuthSecretKeyRef.Name,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "nkey",
			ReadOnly:  true,
			MountPath: NkeyDir,
		})
		envVars = append(envVars, corev1.EnvVar{
			Name:  "NKEY_SEED_FILENAME",
			Value: fmt.Sprintf("%s/seed", NkeyDir),
		})
	}
	return
}

func NatsObjectNameFromURL(url string) string {
	re := regexp.MustCompile("nats://(.+)-nats-client")
	matches := re.FindStringSubmatch(url)
	if matches == nil {
		return ""
	}
	return matches[1]
}
