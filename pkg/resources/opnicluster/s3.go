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
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var s3JsonTemplate = `{
	"identities": [
		{
			"name": "admin",
			"credentials": [
				{
					"accessKey": "%s",
					"secretKey": "%s"
				}
			],
			"actions": ["Admin", "Read", "Write", "List", "Tagging"]
		}
	]
}`

func (r *Reconciler) seaweed() []resources.Resource {
	lg := log.FromContext(r.ctx)
	labels := map[string]string{
		"app": "seaweed",
	}
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-seaweed-s3",
			Namespace: r.opniCluster.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "s3",
					Port:       80,
					TargetPort: intstr.FromInt(8333),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
		},
	}
	workload := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-seaweed",
			Namespace: r.opniCluster.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			ServiceName: "opni-seaweed-s3",
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
			},
		},
	}
	// If internal is disabled, ensure the seaweed pod does not exist
	if r.opniCluster.Spec.S3.Internal == nil {
		return []resources.Resource{
			resources.Absent(workload),
			resources.Absent(service),
		}
	}
	// Otherwise, fill out the pod template and ensure the pod exists

	container := corev1.Container{
		Name:  "seaweed",
		Image: "chrislusf/seaweedfs",
		Ports: []corev1.ContainerPort{
			{
				Name:          "s3",
				ContainerPort: int32(8333),
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
		Env: []corev1.EnvVar{
			{
				Name: "POD_IP",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "status.podIP",
					},
				},
			},
			{
				Name: "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
			{
				Name: "NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
		},
		Args: []string{
			"server",
			"-ip.bind=0.0.0.0",
			"-dir=/var/lib/seaweed/data",
			"-s3",
			"-s3.config=/etc/seaweed/config.json",
			"-s3.allowEmptyFolder=true",
			fmt.Sprintf("-s3.domainName=opni-seaweed-s3.%s.svc", r.opniCluster.Namespace),
			"-s3.port=8333",
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "seaweed-config",
				MountPath: "/etc/seaweed/config.json",
				SubPath:   "config.json",
				ReadOnly:  true,
			},
			{
				Name:      "opni-seaweed-data",
				MountPath: "/var/lib/seaweed/data",
			},
		},
	}
	workload.Spec.Template.Spec.Volumes = []corev1.Volume{
		{
			Name: "seaweed-config",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "opni-seaweed-config",
					Items: []corev1.KeyToPath{
						{
							Key:  "config.json",
							Path: "config.json",
						},
					},
					DefaultMode: pointer.Int32(420),
				},
			},
		},
	}
	if p := r.opniCluster.Spec.S3.Internal.Persistence; p != nil && p.Enabled {
		// Use a persistent volume
		resourceRequest, err := resource.ParseQuantity(p.Request)
		if err != nil {
			lg.Error(err, "failed to parse storage resource request, defaulting to '10Gi'")
			resourceRequest = resource.MustParse("10Gi")
		}
		workload.Spec.Template.Spec.Volumes = append(workload.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "opni-seaweed-data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: "opni-seaweed-data",
				},
			},
		})
		workload.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "opni-seaweed-data",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes:      p.AccessModes,
					StorageClassName: p.StorageClassName,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resourceRequest,
						},
					},
				},
			},
		}
	} else {
		// Use an emptydir volume
		workload.Spec.Template.Spec.Volumes = append(workload.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "opni-seaweed-data",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}
	workload.Spec.Template.Spec.Containers = []corev1.Container{container}

	ctrl.SetControllerReference(r.opniCluster, workload, r.client.Scheme())
	ctrl.SetControllerReference(r.opniCluster, service, r.client.Scheme())

	return []resources.Resource{
		resources.Present(workload),
		resources.Present(service),
	}
}

func (r *Reconciler) internalKeySecret() ([]resources.Resource, error) {
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-seaweed-config",
			Namespace: r.opniCluster.Namespace,
			Labels: map[string]string{
				"app": "seaweed",
			},
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
		accessKey := generateRandomPassword()
		secretKey := generateRandomPassword()
		sec.StringData = map[string]string{
			"accessKey":   string(accessKey),
			"secretKey":   string(secretKey),
			"config.json": fmt.Sprintf(s3JsonTemplate, accessKey, secretKey),
		}
		ctrl.SetControllerReference(r.opniCluster, sec, r.client.Scheme())
		if err := r.client.Create(r.ctx, sec); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	// Update auth status
	if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniCluster), r.opniCluster); err != nil {
		return nil, err
	}
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
		"http://opni-seaweed-s3.%s.svc", r.opniCluster.Namespace)
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
	ns := r.opniCluster.Namespace
	if r.opniCluster.Spec.S3.External.Credentials.Namespace != "" {
		ns = r.opniCluster.Spec.S3.External.Credentials.Namespace
	}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.opniCluster.Spec.S3.External.Credentials.Name,
			Namespace: ns,
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
	if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniCluster), r.opniCluster); err != nil {
		return err
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
	list = append(list, r.seaweed()...)
	return
}

func (r *Reconciler) externalS3() (list []resources.Resource, _ error) {
	return nil, r.externalKeySecret()
}
