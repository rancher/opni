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

package v1beta1

import (
	"fmt"

	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/api/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
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
		Repository: "rancher/mirrored-banzaicloud-fluentd",
		Tag:        "v1.11.5-alpine-21",
	}
	DefaultConfigReloaderImage = loggingv1beta1.ImageSpec{
		Repository: "rancher/mirrored-jimmidyson-configmap-reload",
		Tag:        "v0.4.0",
	}
)

// log is for logging in this package.
var logger = logf.Log.WithName("logadapter-webhook")

func (r *LogAdapter) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-opni-io-v1beta1-logadapter,mutating=true,failurePolicy=fail,sideEffects=None,groups=opni.io,resources=logadapters,verbs=create;update,versions=v1beta1,name=mlogadapter.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Defaulter = &LogAdapter{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *LogAdapter) Default() {
	logger.Info("apply defaults", "name", r.Name)

	if r.Spec.Fluentbit == nil {
		r.Spec.Fluentbit = &loggingv1beta1.FluentbitSpec{}
	}
	if r.Spec.Fluentd == nil {
		r.Spec.Fluentd = &loggingv1beta1.FluentdSpec{}
	}

	// Add our own defaults first
	if r.Spec.Fluentbit.Image.Repository == "" {
		r.Spec.Fluentbit.Image.Repository = DefaultFluentbitImage.Repository
	}
	if r.Spec.Fluentbit.Image.Tag == "" {
		r.Spec.Fluentbit.Image.Tag = DefaultFluentbitImage.Tag
	}
	if r.Spec.Fluentd.Image.Repository == "" {
		r.Spec.Fluentd.Image.Repository = DefaultFluentdImage.Repository
	}
	if r.Spec.Fluentd.Image.Tag == "" {
		r.Spec.Fluentd.Image.Tag = DefaultFluentdImage.Tag
	}
	if r.Spec.Fluentd.ConfigReloaderImage.Repository == "" {
		r.Spec.Fluentd.ConfigReloaderImage.Repository = DefaultConfigReloaderImage.Repository
	}
	if r.Spec.Fluentd.ConfigReloaderImage.Tag == "" {
		r.Spec.Fluentd.ConfigReloaderImage.Tag = DefaultConfigReloaderImage.Tag
	}

	// Apply provider-specific defaults
	r.Spec.Provider.ApplyDefaults(r)

	// Use existing Logging type to set remaining fluentbit and fluentd defaults
	// Any user-specified values will not be overwritten
	(&loggingv1beta1.Logging{
		Spec: loggingv1beta1.LoggingSpec{
			FluentbitSpec: r.Spec.Fluentbit,
			FluentdSpec:   r.Spec.Fluentd,
		},
	}).SetDefaults()
}

//+kubebuilder:webhook:path=/validate-opni-io-v1beta1-logadapter,mutating=false,failurePolicy=fail,sideEffects=None,groups=opni.io,resources=logadapters,verbs=create;update,versions=v1beta1,name=vlogadapter.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &LogAdapter{}

func validateProviderSettings(r *LogAdapter) error {
	fields := map[LogProvider]interface{}{
		LogProviderAKS:       r.Spec.AKS,
		LogProviderEKS:       r.Spec.EKS,
		LogProviderGKE:       r.Spec.GKE,
		LogProviderK3S:       r.Spec.K3S,
		LogProviderRKE:       r.Spec.RKE,
		LogProviderRKE2:      r.Spec.RKE2,
		LogProviderKubeAudit: r.Spec.KubeAudit,
	}

	// Ensure that the provider matches the given settings
	for k, v := range fields {
		if v == nil && r.Spec.Provider == k {
			return field.Required(field.NewPath("spec", "provider"),
				"Provider settings field was not defaulted correctly (is the webhook running?)")
		} else if v != nil && r.Spec.Provider != k {
			return field.Forbidden(field.NewPath("spec", "provider"),
				fmt.Sprintf("Provider is set to %s, but field %s is set", r.Spec.Provider, field.NewPath("spec", string(k))))
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
