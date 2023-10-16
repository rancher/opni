package alerting

import (
	"fmt"
	"path"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	dataMountPath        = "/var/lib"
	webMountPath         = "/etc/web"
	serverCertsMountPath = "/run/certs/server"
	certStore            = "/etc/ssl/certs"
	clientCertsMountPath = "/run/certs/client"
	alertingUser         = "alerting"
	requiredData         = "opni-alertmanager-data"
)

func (r *Reconciler) storage() ([]corev1.PersistentVolumeClaim, []corev1.Volume) {
	pvc, requiredVolumes := r.handlePVC("5Gi")

	requiredVolumes = append(
		requiredVolumes,
		corev1.Volume{
			Name: "alerting-server-certs",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  "alerting-serving-cert-keys",
					DefaultMode: lo.ToPtr[int32](0400),
					Items: []corev1.KeyToPath{
						{
							Key:  "tls.crt",
							Path: "tls.crt",
						},
						{
							Key:  "tls.key",
							Path: "tls.key",
						},
						{
							Key:  "ca.crt",
							Path: "ca.crt",
						},
					},
				},
			},
		},
		corev1.Volume{
			Name: "alerting-web-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "opni-alerting-web-config",
					},
				},
			},
		},
		corev1.Volume{
			Name: "alerting-client-certs",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  "alerting-client-cert-keys",
					DefaultMode: lo.ToPtr[int32](0400),
					Items: []corev1.KeyToPath{
						{
							Key:  "tls.crt",
							Path: "tls.crt",
						},
						{
							Key:  "tls.key",
							Path: "tls.key",
						},
						{
							Key:  "ca.crt",
							Path: "ca.crt",
						},
					},
				},
			},
		},
		corev1.Volume{
			Name: "alerting-server-cacert",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  "alerting-serving-cert-keys",
					DefaultMode: lo.ToPtr[int32](0400),
					Items: []corev1.KeyToPath{
						{
							Key:  "ca.crt",
							Path: "ca.crt",
						},
					},
				},
			},
		},
		corev1.Volume{
			Name: "opni-gateway-serving-cert",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  "opni-gateway-serving-cert",
					DefaultMode: lo.ToPtr[int32](0400),
					Items: []corev1.KeyToPath{
						{
							Key:  "ca.crt",
							Path: "opni-gateway.crt",
						},
					},
				},
			},
		},
	)

	requiredPersistentClaims := []corev1.PersistentVolumeClaim{pvc}
	return requiredPersistentClaims, requiredVolumes
}

func (r *Reconciler) alertmanagerMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      requiredData,
			MountPath: dataMountPath,
		},
		{
			Name:      "alerting-server-certs",
			MountPath: serverCertsMountPath,
		},
		{
			Name:      "alerting-client-certs",
			MountPath: clientCertsMountPath,
		},
		{
			Name:      "alerting-web-config",
			MountPath: webMountPath,
		},
		{
			Name:      "opni-gateway-serving-cert",
			MountPath: certStore,
		},
	}
}

func (r *Reconciler) syncerMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      requiredData,
			MountPath: dataMountPath,
		},
		{
			Name:      "alerting-client-certs",
			MountPath: clientCertsMountPath,
		},
		{
			Name:      "alerting-server-cacert",
			MountPath: serverCertsMountPath,
		},
		{
			Name:      "opni-gateway-serving-cert",
			MountPath: certStore,
		},
	}
}

func (r *Reconciler) webConfig() (*corev1.ConfigMap, error) {

	// prometheus-webkit does something really annoying with marshalling that does not work
	// when we marshal->unmarshal the struct
	httpConfig := fmt.Sprintf(`
tls_server_config:
    cert_file: %s
    key_file: %s
    client_auth_type: ""
    client_ca_file: ""
    cipher_suites: []
    curve_preferences: []
    min_version: TLS12
    max_version: TLS13
    prefer_server_cipher_suites: false
    client_allowed_sans: []
http_server_config:
    http2: false

`,
		path.Join(serverCertsMountPath, "tls.crt"),
		path.Join(serverCertsMountPath, "tls.key"),
		// path.Join(serverCertsMountPath, "ca.crt"),
	)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-alerting-web-config",
			Namespace: r.gw.Namespace,
		},
		Data: map[string]string{
			"web.yml": httpConfig,
		},
	}
	return cm, nil
}

// Similar to "opensearch.opster.io/pkg/builders"
func (r *Reconciler) handlePVC(diskSize string) (pvc corev1.PersistentVolumeClaim, volumes []corev1.Volume) {
	dataVolume := corev1.Volume{}

	node := r.ac.Spec.Alertmanager.ApplicationSpec
	if node.PersistenceConfig == nil || node.PersistenceConfig.PersistenceSource.PVC != nil {
		mode := corev1.PersistentVolumeFilesystem
		pvc = corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: requiredData},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: func() []corev1.PersistentVolumeAccessMode {
					if node.PersistenceConfig == nil {
						return []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
					}
					return node.PersistenceConfig.PersistenceSource.PVC.AccessModes
				}(),
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(diskSize),
					},
				},
				StorageClassName: func() *string {
					if node.PersistenceConfig == nil {
						return nil
					}
					if node.PersistenceConfig.PVC.StorageClassName == "" {
						return nil
					}

					return &node.PersistenceConfig.PVC.StorageClassName
				}(),
				VolumeMode: &mode,
			},
		}
	}

	if node.PersistenceConfig != nil {
		dataVolume.Name = requiredData

		if node.PersistenceConfig.PersistenceSource.HostPath != nil {
			dataVolume.VolumeSource = corev1.VolumeSource{
				HostPath: node.PersistenceConfig.PersistenceSource.HostPath,
			}
			volumes = append(volumes, dataVolume)
		}

		if node.PersistenceConfig.PersistenceSource.EmptyDir != nil {
			dataVolume.VolumeSource = corev1.VolumeSource{
				EmptyDir: node.PersistenceConfig.PersistenceSource.EmptyDir,
			}
			volumes = append(volumes, dataVolume)
		}
	}
	return
}
