package cortex

import (
	"github.com/rancher/opni/pkg/resources"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	deployment  = "deployment"
	statefulSet = "statefulset"

	all           = "all"
	distributor   = "distributor"
	queryFrontend = "query-frontend"
	purger        = "purger"
	ruler         = "ruler"
	compactor     = "compactor"
	storeGateway  = "store-gateway"
	ingester      = "ingester"
	alertmanager  = "alertmanager"
	querier       = "querier"
)

var availableTargets = map[string]string{
	all:           statefulSet,
	distributor:   deployment,
	queryFrontend: deployment,
	purger:        deployment,
	ruler:         deployment,
	compactor:     statefulSet,
	storeGateway:  statefulSet,
	ingester:      statefulSet,
	alertmanager:  statefulSet,
	querier:       statefulSet,
}

var workloadOptions = map[string][]CortexWorkloadOption{
	all: {
		ExtraVolumeMounts(corev1.VolumeMount{
			Name:      "tmp",
			MountPath: "/rules",
		}),
		ExtraVolumes(corev1.Volume{
			Name: "tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}),
		Replicas(1), // Force replicas to 1 for all-in-one mode
		NoLivenessProbe(),
		NoStartupProbe(),
		TerminationGracePeriodSeconds(600),
		VolumeClaimTemplates(corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "data",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("64Gi"), // TODO: Make this configurable
					},
				},
			},
		}),
	},
	distributor: {},
	queryFrontend: {
		Ports(HTTP, GRPC),
	},
	purger: {
		Ports(HTTP),
	},
	ruler: {
		Ports(HTTP, Gossip),
		ExtraVolumeMounts(corev1.VolumeMount{
			Name:      "tmp",
			MountPath: "/rules",
		}),
		ExtraVolumes(corev1.Volume{
			Name: "tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}),
	},
	compactor: {},
	storeGateway: {
		ServiceName("cortex-store-gateway-headless"),
	},
	ingester: {
		Lifecycle(&corev1.Lifecycle{
			PreStop: &corev1.LifecycleHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/ingester/shutdown",
					Port: intstr.FromString("http-metrics"),
				},
			},
		}),
		NoLivenessProbe(),
		NoStartupProbe(),
		TerminationGracePeriodSeconds(600),
		VolumeClaimTemplates(corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "data",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("64Gi"),
					},
				},
			},
		}),
	},
	alertmanager: {
		ExtraVolumes(corev1.Volume{
			Name: "fallback-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "alertmanager-fallback-config",
					},
					Items: []corev1.KeyToPath{
						{
							Key:  "fallback.yaml",
							Path: "fallback.yaml",
						},
					},
				},
			},
		}),
		ExtraVolumeMounts(corev1.VolumeMount{
			Name:      "fallback-config",
			MountPath: "/etc/alertmanager/fallback.yaml",
			SubPath:   "fallback.yaml",
		}),
	},
	querier: {
		Ports(HTTP),
	},
}

func (r *Reconciler) cortexWorkloads(configDigest string) []resources.Resource {
	var res []resources.Resource
	deployedWorkloads := map[string]struct{}{}

	if lo.FromPtr(r.mc.Spec.Cortex.Enabled) {
		for target, spec := range r.mc.Spec.Cortex.CortexWorkloads.GetTargets() {
			workloadType, ok := availableTargets[target]
			if !ok {
				r.logger.With("target", target).Errorf("unknown cortex workload target")
				continue
			}
			opts := append([]CortexWorkloadOption{
				Replicas(spec.GetReplicas()),
				ExtraArgs(spec.GetExtraArgs()...),
				ExtraAnnotations(map[string]string{
					resources.OpniConfigHash: configDigest,
				}),
			}, workloadOptions[target]...)
			switch workloadType {
			case deployment:
				res = append(res, resources.Present(r.buildCortexDeployment(target, opts...)))
			case statefulSet:
				res = append(res, resources.Present(r.buildCortexStatefulSet(target, opts...)))
			}
			deployedWorkloads[target] = struct{}{}
		}
	}

	// ensure remaining unspecified workloads do not exist
	for target := range availableTargets {
		if _, ok := deployedWorkloads[target]; !ok {
			switch availableTargets[target] {
			case deployment:
				res = append(res, resources.Absent(r.buildCortexDeploymentMeta(target)))
			case statefulSet:
				res = append(res, resources.Absent(r.buildCortexStatefulSetMeta(target)))
			}
		}
	}
	return res
}
