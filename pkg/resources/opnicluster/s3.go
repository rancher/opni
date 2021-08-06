package opnicluster

import (
	"fmt"

	opnierrs "github.com/rancher/opni/pkg/errors"
	"github.com/rancher/opni/pkg/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *Reconciler) cephNano() []resources.Resource {
	labels := map[string]string{
		"app":    "ceph",
		"daemon": "nano",
	}
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-ceph-nano-s3",
			Namespace: r.opniCluster.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "s3",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
		},
	}
	workload := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-ceph-nano",
			Namespace: r.opniCluster.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			ServiceName: "opni-ceph-nano-s3",
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
			},
		},
	}
	// If internal is disabled, ensure the ceph nano pod does not exist
	if r.opniCluster.Spec.S3.Internal == nil {
		return []resources.Resource{
			resources.Absent(workload),
			resources.Absent(service),
		}
	}
	// Otherwise, fill out the pod template and ensure the pod exists

	container := corev1.Container{
		Name:  "ceph-nano",
		Image: "ceph/daemon",
		Ports: []corev1.ContainerPort{
			{
				Name:          "s3",
				ContainerPort: int32(8080),
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "web",
				ContainerPort: int32(5000),
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("512M"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("512M"),
			},
		},
		Env: r.cephNanoEnv(),
	}
	workload.Spec.Template.Spec.Containers = []corev1.Container{container}

	ctrl.SetControllerReference(r.opniCluster, workload, r.client.Scheme())
	ctrl.SetControllerReference(r.opniCluster, service, r.client.Scheme())

	return []resources.Resource{
		resources.Present(workload),
		resources.Present(service),
	}
}

func (r *Reconciler) cephNanoEnv() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "CEPH_PUBLIC_NETWORK",
			Value: "0.0.0.0/0",
		},
		{
			Name:  "MON_IP",
			Value: "127.0.0.1",
		},
		{
			Name:  "CEPH_DEMO_UID",
			Value: "opni",
		},
		{
			Name: "CEPH_DEMO_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: r.opniCluster.Status.Auth.S3AccessKey,
			},
		},
		{
			Name: "CEPH_DEMO_SECRET_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: r.opniCluster.Status.Auth.S3SecretKey,
			},
		},
		{
			Name:  "CEPH_DAEMON",
			Value: "demo",
		},
		{
			Name:  "RGW_NAME",
			Value: fmt.Sprintf("opni-ceph-nano-s3.%s.svc", r.opniCluster.Namespace),
		},
	}
}

func (r *Reconciler) internalKeySecret() ([]resources.Resource, error) {
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-internal-s3-keys",
			Namespace: r.opniCluster.Namespace,
		},
	}
	if r.opniCluster.Spec.S3.Internal == nil {
		return []resources.Resource{
			resources.Absent(sec),
		}, nil
	}
	err := r.client.Get(r.ctx, client.ObjectKeyFromObject(sec), sec)
	if errors.IsNotFound(err) {
		// Create the secret
		sec.StringData = map[string]string{
			"accessKey": string(generateRandomPassword()),
			"secretKey": string(generateRandomPassword()),
		}
		if err := r.client.Create(r.ctx, sec); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	// Update auth status
	r.opniCluster.Status.Auth.S3AccessKey = &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: sec.Name,
		},
		Key: "accessKey",
	}
	r.opniCluster.Status.Auth.S3SecretKey = &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: sec.Name,
		},
		Key: "secretKey",
	}
	r.opniCluster.Status.Auth.S3Endpoint = fmt.Sprintf(
		"http://opni-ceph-nano-s3.%s.svc", r.opniCluster.Namespace)
	if err := r.client.Status().Update(r.ctx, r.opniCluster); err != nil {
		return nil, err
	}
	return nil, nil
}

func (r *Reconciler) externalKeySecret() error {
	if r.opniCluster.Spec.S3.External == nil {
		return nil
	}
	if r.opniCluster.Spec.S3.External.Credentials == nil {
		// Credentials not provided, nothing to do
		return fmt.Errorf("%w: external credentials secret not provided",
			opnierrs.ErrS3Credentials)
	}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.opniCluster.Spec.S3.External.Credentials.Name,
			Namespace: r.opniCluster.Namespace,
		},
	}
	err := r.client.Get(r.ctx, client.ObjectKeyFromObject(sec), sec)
	if errors.IsNotFound(err) {
		return fmt.Errorf("%w: secret must already exist in the same namespace as the opnicluster",
			opnierrs.ErrS3Credentials)
	} else if err != nil {
		return err
	}
	if _, ok := sec.Data["accessKey"]; !ok {
		return fmt.Errorf("%w: secret must contain an item named accessKey",
			opnierrs.ErrS3Credentials)
	} else {
		r.opniCluster.Status.Auth.S3AccessKey = &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: sec.Name,
			},
			Key: "accessKey",
		}
	}
	if _, ok := sec.Data["secretKey"]; !ok {
		return fmt.Errorf("%w: secret must contain an item named secretKey",
			opnierrs.ErrS3Credentials)
	} else {
		r.opniCluster.Status.Auth.S3SecretKey = &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: sec.Name,
			},
			Key: "secretKey",
		}
	}
	r.opniCluster.Status.Auth.S3Endpoint = r.opniCluster.Spec.S3.External.Endpoint
	if err := r.client.Status().Update(r.ctx, r.opniCluster); err != nil {
		return err
	}
	return nil
}

func (r *Reconciler) internalS3() (list []resources.Resource, _ error) {
	items, err := r.internalKeySecret()
	list = append(list, items...)
	if err != nil {
		return nil, err
	}
	list = append(list, r.cephNano()...)
	return
}

func (r *Reconciler) externalS3() (list []resources.Resource, _ error) {
	return nil, r.externalKeySecret()
}
