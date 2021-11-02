package thanos

import (
	"github.com/banzaicloud/operator-tools/pkg/secret"
	"github.com/banzaicloud/operator-tools/pkg/volume"
	thanosv1alpha1 "github.com/banzaicloud/thanos-operator/pkg/sdk/api/v1alpha1"
	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *Reconciler) thanosWorkloads() []resources.Resource {
	objectStore := r.objectStore()
	storeEndpoint := r.storeEndpoint()
	thanos := r.thanos()
	if pointer.BoolDeref(r.opniCluster.Spec.Services.Metrics.Enabled, false) {
		return []resources.Resource{
			resources.Present(objectStore),
			resources.Present(storeEndpoint),
			resources.Present(thanos),
		}
	}
	return []resources.Resource{
		resources.Absent(objectStore),
		resources.Absent(storeEndpoint),
		resources.Absent(thanos),
	}
}

func (r *Reconciler) objectStore() client.Object {
	bucketWeb := thanosv1alpha1.DefaultBucketWeb.DeepCopy()
	compactor := thanosv1alpha1.DefaultCompactor.DeepCopy()
	if r.opniCluster.Spec.Thanos.CompactorPersistence != nil {
		compactor.DataDir = "/var/lib/compactor/data"
		compactor.DataVolume = &volume.KubernetesVolume{
			PersistentVolumeClaim: &volume.PersistentVolumeClaim{
				PersistentVolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: r.opniCluster.Spec.Thanos.CompactorPersistence.AccessModes,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: r.opniCluster.Spec.Thanos.CompactorPersistence.Request,
						},
					},
					StorageClassName: r.opniCluster.Spec.Thanos.CompactorPersistence.StorageClassName,
				},
			},
		}
	}
	os := &thanosv1alpha1.ObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-thanos-objectstore",
			Namespace: r.opniCluster.Spec.Services.Metrics.PrometheusNamespace,
		},
		Spec: thanosv1alpha1.ObjectStoreSpec{
			BucketWeb: bucketWeb,
			Compactor: compactor,
			Config: secret.Secret{
				MountFrom: &secret.ValueFrom{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "opni-thanos-objectstore",
						},
						Key: "object-store.yaml",
					},
				},
			},
		},
	}
	ctrl.SetControllerReference(os, r.opniCluster, r.client.Scheme())
	return os
}

func (r *Reconciler) storeEndpoint() client.Object {
	se := &thanosv1alpha1.StoreEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-thanos-storeendpoint",
			Namespace: r.opniCluster.Spec.Services.Metrics.PrometheusNamespace,
		},
		Spec: thanosv1alpha1.StoreEndpointSpec{
			Thanos:   "opni-thanos",
			Selector: &thanosv1alpha1.KubernetesSelector{},
			Config: secret.Secret{
				MountFrom: &secret.ValueFrom{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "opni-thanos-objectstore",
						},
						Key: "object-store.yaml",
					},
				},
			},
		},
	}
	ctrl.SetControllerReference(se, r.opniCluster, r.client.Scheme())
	return se
}

func (r *Reconciler) thanos() client.Object {
	query := thanosv1alpha1.DefaultQuery.DeepCopy()
	rule := thanosv1alpha1.DefaultRule.DeepCopy()
	storeGateway := thanosv1alpha1.DefaultStoreGateway.DeepCopy()
	thanos := &thanosv1alpha1.Thanos{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-thanos",
			Namespace: r.opniCluster.Spec.Services.Metrics.PrometheusNamespace,
		},
		Spec: thanosv1alpha1.ThanosSpec{
			EnableRecreateWorkloadOnImmutableFieldChange: true,
			Query:        query,
			Rule:         rule,
			StoreGateway: storeGateway,
		},
	}
	ctrl.SetControllerReference(thanos, r.opniCluster, r.client.Scheme())
	return thanos
}
