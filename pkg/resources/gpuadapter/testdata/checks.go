package testdata

import (
	"strings"

	. "github.com/onsi/gomega"
	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/providers"
	"github.com/rancher/opni/pkg/resources/gpuadapter"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func init() {
	constants, err := gpuadapter.BuildClusterPolicy(&v1beta1.GpuPolicyAdapter{}, providers.Unknown)
	if err != nil {
		panic(err)
	}
	constDaemonsets := constants.Spec.Daemonsets
	constDcgmExporterEnv := constants.Spec.DCGMExporter.Env
	constDevicePluginEnv := constants.Spec.DevicePlugin.Env
	constDriverEnv := constants.Spec.Driver.Env
	constGfdEnv := constants.Spec.GPUFeatureDiscovery.Env
	constPrivileged := &corev1.SecurityContext{
		Privileged: pointer.Bool(true),
		SELinuxOptions: &corev1.SELinuxOptions{
			Level: "s0",
		},
	}

	If(func(TestItem) bool {
		return true
	}).Then(func(ti TestItem) {
		Expect(ti.Output.Name).To(Equal(ti.Input.Name))
		Expect(ti.Output.Namespace).To(Equal(ti.Input.Namespace))
		Expect(ti.Output.OwnerReferences).To(ContainElement(metav1.OwnerReference{
			APIVersion:         ti.Input.APIVersion,
			Kind:               ti.Input.Kind,
			Name:               ti.Input.Name,
			UID:                ti.Input.UID,
			Controller:         pointer.Bool(true),
			BlockOwnerDeletion: pointer.Bool(true),
		}))

		// Daemonsets config is constant
		Expect(ti.Output.Spec.Daemonsets).To(Equal(constDaemonsets))

		// Some env vars are constant
		Expect(ti.Output.Spec.DCGMExporter.Env).To(Equal(constDcgmExporterEnv))
		Expect(ti.Output.Spec.DevicePlugin.Env).To(Equal(constDevicePluginEnv))
		Expect(ti.Output.Spec.Driver.Env).To(Equal(constDriverEnv))
		Expect(ti.Output.Spec.GPUFeatureDiscovery.Env).To(Equal(constGfdEnv))

		// Some components are always privileged
		Expect(ti.Output.Spec.Driver.SecurityContext).To(Equal(constPrivileged))
		Expect(ti.Output.Spec.MIGManager.SecurityContext).To(Equal(constPrivileged))
		Expect(ti.Output.Spec.Toolkit.SecurityContext).To(Equal(constPrivileged))
		Expect(ti.Output.Spec.Validator.SecurityContext).To(Equal(constPrivileged))

		// Some components are always enabled or disabled
		truePtr := pointer.Bool(true)
		falsePtr := pointer.Bool(false)
		Expect(ti.Output.Spec.DCGM.Enabled).To(Equal(truePtr))
		Expect(ti.Output.Spec.Driver.Enabled).To(Equal(truePtr))
		Expect(ti.Output.Spec.Driver.GPUDirectRDMA.Enabled).To(Equal(falsePtr))
		Expect(ti.Output.Spec.MIGManager.Enabled).To(Equal(falsePtr))
		Expect(ti.Output.Spec.NodeStatusExporter.Enabled).To(Equal(truePtr))
		Expect(ti.Output.Spec.PSP.Enabled).To(Equal(falsePtr))
		Expect(ti.Output.Spec.Toolkit.Enabled).To(Equal(truePtr))
	})

	If(func(ti TestItem) bool {
		return ti.Input.Spec.ContainerRuntime == v1beta1.Auto
	}).Then(func(ti TestItem) {
		Expect(ti.Output.Spec.Operator.DefaultRuntime).To(
			BeEquivalentTo(ti.Provider.ContainerRuntime()))
	})

	If(func(ti TestItem) bool {
		return ti.Input.Spec.ContainerRuntime == v1beta1.Docker ||
			ti.Input.Spec.ContainerRuntime == v1beta1.Containerd ||
			ti.Input.Spec.ContainerRuntime == v1beta1.Crio
	}).Then(func(ti TestItem) {
		Expect(ti.Output.Spec.Operator.DefaultRuntime).To(
			BeEquivalentTo(ti.Input.Spec.ContainerRuntime))
	})

	IfAndOnlyIf(func(ti TestItem) bool {
		return string(ti.Output.Spec.Operator.DefaultRuntime) == string(v1beta1.Containerd)
	}).Then(func(ti TestItem) {
		Expect(ti.Output.Spec.Toolkit.Env).To(ContainElements(
			corev1.EnvVar{
				Name:  "CONTAINERD_RUNTIME_CLASS",
				Value: "nvidia",
			},
			corev1.EnvVar{
				Name:  "CONTAINERD_SET_AS_DEFAULT",
				Value: "false",
			},
		))
	})

	IfAndOnlyIf(func(ti TestItem) bool {
		return string(ti.Output.Spec.Operator.DefaultRuntime) == string(v1beta1.Containerd) &&
			ti.Provider == providers.K3S
	}).Then(func(ti TestItem) {
		Expect(ti.Output.Spec.Toolkit.Env).To(ContainElements(
			corev1.EnvVar{
				Name:  "CONTAINERD_CONFIG",
				Value: "/var/lib/rancher/k3s/agent/etc/containerd/config.toml",
			},
			corev1.EnvVar{
				Name:  "CONTAINERD_SOCKET",
				Value: "/run/k3s/containerd/containerd.sock",
			},
		))
	})

	IfAndOnlyIf(func(ti TestItem) bool {
		return string(ti.Output.Spec.Operator.DefaultRuntime) == string(v1beta1.Containerd) &&
			ti.Provider == providers.RKE2
	}).Then(func(ti TestItem) {
		Expect(ti.Output.Spec.Toolkit.Env).To(ContainElements(
			corev1.EnvVar{
				Name:  "CONTAINERD_CONFIG",
				Value: "/var/lib/rancher/rke2/agent/etc/containerd/config.toml",
			},
			corev1.EnvVar{
				Name:  "CONTAINERD_SOCKET",
				Value: "/run/rke2/containerd/containerd.sock",
			},
		))
	})

	If(func(ti TestItem) bool {
		return ti.Input.Spec.Images.Driver != ""
	}).Then(func(ti TestItem) {
		Expect(ti.Output.Spec.Driver.Image).To(Equal(ti.Input.Spec.Images.Driver))
	}).Else(func(ti TestItem) {
		Expect(ti.Output.Spec.Driver.Image).To(Equal(gpuadapter.DefaultImages.Driver))
	})

	If(func(ti TestItem) bool {
		return ti.Input.Spec.Images.DriverManager != ""
	}).Then(func(ti TestItem) {
		Expect(ti.Output.Spec.Driver.Manager.Image).To(Equal(ti.Input.Spec.Images.DriverManager))
	}).Else(func(ti TestItem) {
		Expect(ti.Output.Spec.Driver.Manager.Image).To(Equal(gpuadapter.DefaultImages.DriverManager))
	})

	If(func(ti TestItem) bool {
		return ti.Input.Spec.Images.DCGM != ""
	}).Then(func(ti TestItem) {
		Expect(ti.Output.Spec.DCGM.Image).To(Equal(ti.Input.Spec.Images.DCGM))
	}).Else(func(ti TestItem) {
		Expect(ti.Output.Spec.DCGM.Image).To(Equal(gpuadapter.DefaultImages.DCGM))
	})

	If(func(ti TestItem) bool {
		return ti.Input.Spec.Images.DCGMExporter != ""
	}).Then(func(ti TestItem) {
		Expect(ti.Output.Spec.DCGMExporter.Image).To(Equal(ti.Input.Spec.Images.DCGMExporter))
	}).Else(func(ti TestItem) {
		Expect(ti.Output.Spec.DCGMExporter.Image).To(Equal(gpuadapter.DefaultImages.DCGMExporter))
	})

	If(func(ti TestItem) bool {
		return ti.Input.Spec.Images.DevicePlugin != ""
	}).Then(func(ti TestItem) {
		Expect(ti.Output.Spec.DevicePlugin.Image).To(Equal(ti.Input.Spec.Images.DevicePlugin))
	}).Else(func(ti TestItem) {
		Expect(ti.Output.Spec.DevicePlugin.Image).To(Equal(gpuadapter.DefaultImages.DevicePlugin))
	})

	If(func(ti TestItem) bool {
		return ti.Input.Spec.Images.GFD != ""
	}).Then(func(ti TestItem) {
		Expect(ti.Output.Spec.GPUFeatureDiscovery.Image).To(Equal(ti.Input.Spec.Images.GFD))
	}).Else(func(ti TestItem) {
		Expect(ti.Output.Spec.GPUFeatureDiscovery.Image).To(Equal(gpuadapter.DefaultImages.GFD))
	})

	If(func(ti TestItem) bool {
		return ti.Input.Spec.Images.InitContainer != ""
	}).Then(func(ti TestItem) {
		Expect(ti.Output.Spec.Operator.InitContainer.Image).To(Equal(ti.Input.Spec.Images.InitContainer))
	}).Else(func(ti TestItem) {
		Expect(ti.Output.Spec.Operator.InitContainer.Image).To(Equal(gpuadapter.DefaultImages.InitContainer))
	})

	If(func(ti TestItem) bool {
		return ti.Input.Spec.Images.Toolkit != ""
	}).Then(func(ti TestItem) {
		Expect(ti.Output.Spec.Toolkit.Image).To(Equal(ti.Input.Spec.Images.Toolkit))
	}).Else(func(ti TestItem) {
		Expect(ti.Output.Spec.Toolkit.Image).To(Equal(gpuadapter.DefaultImages.Toolkit))
	})

	If(func(ti TestItem) bool {
		return ti.Input.Spec.Images.Validator != ""
	}).Then(func(ti TestItem) {
		Expect(ti.Output.Spec.Validator.Image).To(Equal(ti.Input.Spec.Images.Validator))
	}).Else(func(ti TestItem) {
		Expect(ti.Output.Spec.Validator.Image).To(Equal(gpuadapter.DefaultImages.Validator))
	})

	If(func(ti TestItem) bool {
		return ti.Input.Spec.Images.MIGManager != ""
	}).Then(func(ti TestItem) {
		Expect(ti.Output.Spec.MIGManager.Image).To(Equal(ti.Input.Spec.Images.MIGManager))
	}).Else(func(ti TestItem) {
		Expect(ti.Output.Spec.MIGManager.Image).To(Equal(gpuadapter.DefaultImages.MIGManager))
	})

	IfAndOnlyIf(func(ti TestItem) bool {
		return ti.Input.Spec.VGPU != nil
	}).Then(func(ti TestItem) {
		Expect(ti.Output.Spec.Driver.LicensingConfig).NotTo(BeNil())
		Expect(ti.Output.Spec.Driver.LicensingConfig.ConfigMapName).To(
			Equal(ti.Input.Spec.VGPU.LicenseConfigMap))
		Expect(ti.Output.Spec.Driver.LicensingConfig.NLSEnabled).To(
			Equal(pointer.Bool(strings.ToLower(ti.Input.Spec.VGPU.LicenseServerKind) == "nls")))
	})
}
