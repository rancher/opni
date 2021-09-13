package gpuadapter

import (
	"context"
	"fmt"

	nvidiav1 "github.com/NVIDIA/gpu-operator/api/v1"
	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/imdario/mergo"
	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/providers"
	"github.com/rancher/opni/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	defaultImages = v1beta1.ImagesSpec{
		Driver:        "nvcr.io/nvidia/driver:470.57.02",
		DriverManager: "nvcr.io/nvidia/cloud-native/k8s-driver-manager:v0.1.0",
		DCGM:          "nvcr.io/nvidia/cloud-native/dcgm:2.2.3-ubuntu20.04",
		DCGMExporter:  "nvcr.io/nvidia/k8s/dcgm-exporter:2.2.9-2.4.0-ubuntu20.04",
		DevicePlugin:  "nvcr.io/nvidia/k8s-device-plugin:v0.9.0-ubuntu20.04",
		GFD:           "nvcr.io/nvidia/gpu-feature-discovery:v0.4.1",
		InitContainer: "nvcr.io/nvidia/cuda:11.2.1-base-ubuntu20.04",
		MIGManager:    "nvcr.io/nvidia/cloud-native/k8s-mig-manager:v0.1.2-ubuntu20.04",
		Toolkit:       "joekralicky/container-toolkit:1.7.0-ubuntu20.04",
		Validator:     "joekralicky/gpu-operator-validator:v1.8.1-ubuntu20.04",
	}
)

func ReconcileGPUAdapter(
	ctx context.Context,
	cli client.Client,
	gpa *v1beta1.GpuPolicyAdapter,
) (ctrl.Result, error) {
	lg := log.FromContext(ctx)
	rec := reconciler.NewReconcilerWith(cli,
		reconciler.WithLog(lg),
		reconciler.WithScheme(cli.Scheme()),
	)
	provider, err := providers.Detect(ctx, cli)
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	policy, err := buildClusterPolicy(gpa, provider)
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	err = mergo.Merge(&policy.Spec, gpa.Spec.Template)
	if err != nil {
		return util.RequeueErr(err).Result()
	}

	return util.LoadResult(rec.ReconcileResource(policy, reconciler.StatePresent)).Result()
}

func buildClusterPolicy(
	gpa *v1beta1.GpuPolicyAdapter,
	provider providers.Provider,
) (*nvidiav1.ClusterPolicy, error) {
	mergo.Merge(&gpa.Spec.Images, &defaultImages)
	policy := &nvidiav1.ClusterPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gpa.Name,
			Namespace: gpa.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: gpa.APIVersion,
					Kind:       gpa.Kind,
					Name:       gpa.Name,
					UID:        gpa.UID,
					Controller: pointer.BoolPtr(true),
				},
			},
		},
		Spec: nvidiav1.ClusterPolicySpec{
			Daemonsets: nvidiav1.DaemonsetsSpec{
				PriorityClassName: "system-node-critical",
				Tolerations: []corev1.Toleration{
					{
						Key:      "nvidia.com/gpu",
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
			},
			DCGM: nvidiav1.DCGMSpec{
				Enabled:         pointer.Bool(true),
				HostPort:        5555,
				ImagePullPolicy: string(corev1.PullIfNotPresent),
				Image:           gpa.Spec.Images.DCGM,
			},
			DCGMExporter: nvidiav1.DCGMExporterSpec{
				Env: []corev1.EnvVar{
					{
						Name:  "DCGM_EXPORTER_LISTEN",
						Value: ":9400",
					},
					{
						Name:  "DCGM_EXPORTER_KUBERNETES",
						Value: "true",
					},
					{
						Name:  "DCGM_EXPORTER_COLLECTORS",
						Value: "/etc/dcgm-exporter/dcp-metrics-included.csv",
					},
				},
				Image: gpa.Spec.Images.DCGMExporter,
			},
			DevicePlugin: nvidiav1.DevicePluginSpec{
				Env: []corev1.EnvVar{
					{
						Name:  "PASS_DEVICE_SPECS",
						Value: "true",
					},
					{
						Name:  "FAIL_ON_INIT_ERROR",
						Value: "true",
					},
					{
						Name:  "DEVICE_LIST_STRATEGY",
						Value: "envvar",
					},
					{
						Name:  "DEVICE_ID_STRATEGY",
						Value: "uuid",
					},
					{
						Name:  "NVIDIA_VISIBLE_DEVICES",
						Value: "all",
					},
					{
						Name:  "NVIDIA_DRIVER_CAPABILITIES",
						Value: "compute,utility",
					},
				},
				SecurityContext: &corev1.SecurityContext{
					Privileged: pointer.Bool(true),
				},
				Image: gpa.Spec.Images.DevicePlugin,
			},
			Driver: nvidiav1.DriverSpec{
				Enabled: pointer.Bool(true),
				Manager: nvidiav1.DriverManagerSpec{
					Env: []corev1.EnvVar{
						{
							Name:  "ENABLE_AUTO_DRAIN",
							Value: "true",
						},
						{
							Name:  "DRAIN_USE_FORCE",
							Value: "false",
						},
						{
							Name:  "DRAIN_POD_SELECTOR_LABEL",
							Value: "",
						},
						{
							Name:  "DRAIN_TIMEOUT_SECONDS",
							Value: "0",
						},
						{
							Name:  "DRAIN_DELETE_EMPTYDIR_DATA",
							Value: "true",
						},
					},
					Image: gpa.Spec.Images.DriverManager,
				},
				GPUDirectRDMA: &nvidiav1.GPUDirectRDMASpec{
					Enabled: pointer.Bool(false),
				},
				SecurityContext: &corev1.SecurityContext{
					Privileged: pointer.Bool(true),
					SELinuxOptions: &corev1.SELinuxOptions{
						Level: "s0",
					},
				},
				Image: gpa.Spec.Images.Driver,
			},
			GPUFeatureDiscovery: nvidiav1.GPUFeatureDiscoverySpec{
				Env: []corev1.EnvVar{
					{
						Name:  "GFD_SLEEP_INTERVAL",
						Value: "60s",
					},
					{
						Name:  "GFD_FAIL_ON_INIT_ERROR",
						Value: "true",
					},
				},
				Image: gpa.Spec.Images.GFD,
			},
			MIG: nvidiav1.MIGSpec{
				Strategy: nvidiav1.MIGStrategyNone,
			},
			MIGManager: nvidiav1.MIGManagerSpec{
				Enabled: pointer.Bool(false),
				Image:   gpa.Spec.Images.MIGManager,
				Env: []corev1.EnvVar{
					{
						Name:  "WITH_REBOOT",
						Value: "false",
					},
				},
				SecurityContext: &corev1.SecurityContext{
					Privileged: pointer.Bool(true),
				},
			},
			NodeStatusExporter: nvidiav1.NodeStatusExporterSpec{
				Enabled: pointer.Bool(true),
				Image:   gpa.Spec.Images.Validator,
			},
			Operator: nvidiav1.OperatorSpec{
				DefaultRuntime: nvidiav1.Runtime(provider.ContainerRuntime()),
				InitContainer: nvidiav1.InitContainerSpec{
					Image: gpa.Spec.Images.InitContainer,
				},
			},
			PSP: nvidiav1.PSPSpec{
				Enabled: pointer.Bool(false),
			},
			Toolkit: nvidiav1.ToolkitSpec{
				Enabled: pointer.Bool(true),
				Env: func() (vars []corev1.EnvVar) {
					if provider.ContainerRuntime() == "containerd" {
						vars = append(vars, corev1.EnvVar{
							Name:  "CONTAINERD_RUNTIME_CLASS",
							Value: "nvidia",
						}, corev1.EnvVar{
							Name:  "CONTAINERD_SET_AS_DEFAULT",
							Value: "false",
						})
						if provider == providers.K3S || provider == providers.RKE2 {
							vars = append(vars, corev1.EnvVar{
								Name:  "CONTAINERD_CONFIG",
								Value: fmt.Sprintf("/var/lib/rancher/%s/agent/etc/containerd/config.toml", provider.String()),
							}, corev1.EnvVar{
								Name:  "CONTAINERD_SOCKET",
								Value: fmt.Sprintf("/run/%s/containerd/containerd.sock", provider.String()),
							})
						}
					}
					return
				}(),
				SecurityContext: &corev1.SecurityContext{
					Privileged: pointer.Bool(true),
					SELinuxOptions: &corev1.SELinuxOptions{
						Level: "s0",
					},
				},
				Image: gpa.Spec.Images.Toolkit,
			},
			Validator: nvidiav1.ValidatorSpec{
				Plugin: nvidiav1.PluginValidatorSpec{
					Env: []corev1.EnvVar{
						{
							Name:  "WITH_WORKLOAD",
							Value: "true",
						},
					},
				},
				SecurityContext: &corev1.SecurityContext{
					Privileged: pointer.Bool(true),
					SELinuxOptions: &corev1.SELinuxOptions{
						Level: "s0",
					},
				},
				Image: gpa.Spec.Images.Validator,
			},
		},
	}
	return policy, nil
}
