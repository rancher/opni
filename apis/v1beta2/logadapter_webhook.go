/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta2

import (
	"fmt"

	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/rancher/opni/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	DefaultFluentbitImage = loggingv1beta1.ImageSpec{
		Repository: "rancher/mirrored-fluent-fluent-bit",
		Tag:        "1.7.4",
	}
	DefaultFluentdImage = loggingv1beta1.ImageSpec{
		Repository: "quay.io/dbason/banzaicloud-fluentd",
		Tag:        "alpine-1.13-2",
	}
	DefaultConfigReloaderImage = loggingv1beta1.ImageSpec{
		Repository: "rancher/mirrored-jimmidyson-configmap-reload",
		Tag:        "v0.4.0",
	}
	DefaultContainerLogDir = "/var/lib/docker/containers"
	DefaultLivenessProbe   = corev1.Probe{
		InitialDelaySeconds: 30,
		PeriodSeconds:       15,
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt(24240),
			},
		},
	}
)

// log is for logging in this package.
var logger = logf.Log.WithName("logadapter-webhook")

func (r *LogAdapter) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-opni-io-v1beta2-logadapter,mutating=true,failurePolicy=fail,sideEffects=None,groups=opni.io,resources=logadapters,verbs=create;update,versions=v1beta2,name=mlogadapter.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Defaulter = &LogAdapter{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *LogAdapter) Default() {
	logger.Info("apply defaults", "name", r.Name)
	if r.Spec.FluentConfig == nil {
		r.Spec.FluentConfig = &FluentConfigSpec{}
	}
	if r.Spec.RootFluentConfig == nil {
		r.Spec.RootFluentConfig = &FluentConfigSpec{}
	}

	if r.Spec.FluentConfig.Fluentbit == nil {
		r.Spec.FluentConfig.Fluentbit = &loggingv1beta1.FluentbitSpec{}
	}
	if r.Spec.RootFluentConfig.Fluentbit == nil {
		r.Spec.RootFluentConfig.Fluentbit = &loggingv1beta1.FluentbitSpec{}
	}
	if r.Spec.FluentConfig.Fluentd == nil {
		r.Spec.FluentConfig.Fluentd = &loggingv1beta1.FluentdSpec{}
		r.Spec.FluentConfig.Fluentd.DisablePvc = true
		r.Spec.FluentConfig.Fluentd.LivenessProbe = &DefaultLivenessProbe
	}
	if r.Spec.RootFluentConfig.Fluentd == nil {
		r.Spec.RootFluentConfig.Fluentd = &loggingv1beta1.FluentdSpec{}
		r.Spec.RootFluentConfig.Fluentd.DisablePvc = true
		r.Spec.RootFluentConfig.Fluentd.LivenessProbe = &DefaultLivenessProbe
	}

	// Add our own defaults first
	if r.Spec.FluentConfig.Fluentbit.Image.Repository == "" {
		r.Spec.FluentConfig.Fluentbit.Image.Repository = DefaultFluentbitImage.Repository
	}
	if r.Spec.RootFluentConfig.Fluentbit.Image.Repository == "" {
		r.Spec.RootFluentConfig.Fluentbit.Image.Repository = DefaultFluentbitImage.Repository
	}
	if r.Spec.FluentConfig.Fluentbit.Image.Tag == "" {
		r.Spec.FluentConfig.Fluentbit.Image.Tag = DefaultFluentbitImage.Tag
	}
	if r.Spec.RootFluentConfig.Fluentbit.Image.Tag == "" {
		r.Spec.RootFluentConfig.Fluentbit.Image.Tag = DefaultFluentbitImage.Tag
	}
	if r.Spec.FluentConfig.Fluentd.Image.Repository == "" {
		r.Spec.FluentConfig.Fluentd.Image.Repository = DefaultFluentdImage.Repository
	}
	if r.Spec.RootFluentConfig.Fluentd.Image.Repository == "" {
		r.Spec.RootFluentConfig.Fluentd.Image.Repository = DefaultFluentdImage.Repository
	}
	if r.Spec.FluentConfig.Fluentd.Image.Tag == "" {
		r.Spec.FluentConfig.Fluentd.Image.Tag = DefaultFluentdImage.Tag
	}
	if r.Spec.RootFluentConfig.Fluentd.Image.Tag == "" {
		r.Spec.RootFluentConfig.Fluentd.Image.Tag = DefaultFluentdImage.Tag
	}
	if r.Spec.FluentConfig.Fluentd.ConfigReloaderImage.Repository == "" {
		r.Spec.FluentConfig.Fluentd.ConfigReloaderImage.Repository = DefaultConfigReloaderImage.Repository
	}
	if r.Spec.RootFluentConfig.Fluentd.ConfigReloaderImage.Repository == "" {
		r.Spec.RootFluentConfig.Fluentd.ConfigReloaderImage.Repository = DefaultConfigReloaderImage.Repository
	}
	if r.Spec.FluentConfig.Fluentd.ConfigReloaderImage.Tag == "" {
		r.Spec.FluentConfig.Fluentd.ConfigReloaderImage.Tag = DefaultConfigReloaderImage.Tag
	}
	if r.Spec.RootFluentConfig.Fluentd.ConfigReloaderImage.Tag == "" {
		r.Spec.RootFluentConfig.Fluentd.ConfigReloaderImage.Tag = DefaultConfigReloaderImage.Tag
	}
	if r.Spec.ContainerLogDir == "" {
		r.Spec.ContainerLogDir = DefaultContainerLogDir
	}

	// Apply provider-specific defaults
	r.Spec.Provider.ApplyDefaults(r)

	// Use existing Logging type to set remaining fluentbit and fluentd defaults
	// Any user-specified values will not be overwritten
	(&loggingv1beta1.Logging{
		Spec: loggingv1beta1.LoggingSpec{
			FluentbitSpec: r.Spec.FluentConfig.Fluentbit,
			FluentdSpec:   r.Spec.FluentConfig.Fluentd,
		},
	}).SetDefaults()
	(&loggingv1beta1.Logging{
		Spec: loggingv1beta1.LoggingSpec{
			FluentbitSpec: r.Spec.RootFluentConfig.Fluentbit,
			FluentdSpec:   r.Spec.RootFluentConfig.Fluentd,
		},
	}).SetDefaults()
}

//+kubebuilder:webhook:path=/validate-opni-io-v1beta2-logadapter,mutating=false,failurePolicy=fail,sideEffects=None,groups=opni.io,resources=logadapters,verbs=create;update,versions=v1beta2,name=vlogadapter.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &LogAdapter{}

func validateProviderSettings(r *LogAdapter) error {
	fields := map[LogProvider]interface{}{
		LogProviderAKS:  r.Spec.AKS,
		LogProviderEKS:  r.Spec.EKS,
		LogProviderGKE:  r.Spec.GKE,
		LogProviderK3S:  r.Spec.K3S,
		LogProviderRKE:  r.Spec.RKE,
		LogProviderRKE2: r.Spec.RKE2,
	}

	// Ensure that the provider matches the given settings
	for k, v := range fields {
		if (v == nil || util.IsInterfaceNil(v)) && r.Spec.Provider == k {
			return field.Required(field.NewPath("spec", "provider"),
				"Provider settings field was not defaulted correctly (is the webhook running?)")
		} else if !(v == nil || util.IsInterfaceNil(v)) && r.Spec.Provider != k {
			return field.Forbidden(field.NewPath("spec", "provider"),
				fmt.Sprintf("Provider is set to %s, but field %s is set, value is %v", r.Spec.Provider, field.NewPath("spec", string(k)), v))
		}
	}

	return nil
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *LogAdapter) ValidateCreate() error {
	logger.Info("validate create", "name", r.Name)

	return validateProviderSettings(r)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *LogAdapter) ValidateUpdate(old runtime.Object) error {
	logger.Info("validate update", "name", r.Name)
	oldLogAdapter := old.(*LogAdapter)
	if r.Spec.Provider != oldLogAdapter.Spec.Provider {
		return field.Forbidden(field.NewPath("spec", "provider"), "spec.provider cannot be modified once set.")
	}
	return validateProviderSettings(r)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *LogAdapter) ValidateDelete() error {
	return nil // unused for now
}
