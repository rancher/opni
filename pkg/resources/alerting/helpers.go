package alerting

import (
	bucket_http "github.com/cortexproject/cortex/pkg/storage/bucket/http"
	kyamlv3 "github.com/kralicky/yaml/v3"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	fallbackKey   = "fallback.yaml"
	mainKey       = "cortex.yaml"
	dataMountPath = "/var/lib"
	alertingUser  = "alerting"
	requiredData  = "opni-alertmanager-data"
)

func valueOrDefault[T any](t *T) (_ T) {
	if t == nil {
		return
	}
	return *t
}

type overrideMarshaler[T kyamlv3.Marshaler] struct {
	fn func(T) (interface{}, error)
}

func (m *overrideMarshaler[T]) MarshalYAML(v interface{}) (interface{}, error) {
	return m.fn(v.(T))
}

func newOverrideMarshaler[T kyamlv3.Marshaler](fn func(T) (interface{}, error)) *overrideMarshaler[T] {
	return &overrideMarshaler[T]{
		fn: fn,
	}
}

func bucketHttpConfig(spec *storagev1.HTTPConfig) bucket_http.Config {
	return bucket_http.Config{
		IdleConnTimeout:       spec.GetIdleConnTimeout().AsDuration(),
		ResponseHeaderTimeout: spec.GetResponseHeaderTimeout().AsDuration(),
		InsecureSkipVerify:    spec.GetInsecureSkipVerify(),
		TLSHandshakeTimeout:   spec.GetTlsHandshakeTimeout().AsDuration(),
		ExpectContinueTimeout: spec.GetExpectContinueTimeout().AsDuration(),
		MaxIdleConns:          int(spec.GetMaxIdleConns()),
		MaxIdleConnsPerHost:   int(spec.GetMaxIdleConnsPerHost()),
		MaxConnsPerHost:       int(spec.GetMaxConnsPerHost()),
	}
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
