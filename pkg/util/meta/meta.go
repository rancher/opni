// +kubebuilder:object:generate=true

// meta package contains generic kubernetes type implementations for Opni.

package meta

import (
	"fmt"
	"path"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
)

const (
	OpensearchFinalizer = "opni.io/opensearch"
)

type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

type ImageSpec struct {
	Image            *string                       `json:"image,omitempty"`
	ImagePullPolicy  *corev1.PullPolicy            `json:"imagePullPolicy,omitempty"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

func (s ImageSpec) GetImagePullPolicy() (_ corev1.PullPolicy) {
	if p := s.ImagePullPolicy; p != nil {
		return *p
	}
	return
}

func (s ImageSpec) GetImage() string {
	if s.Image == nil {
		return ""
	}
	return *s.Image
}

func (s *ImageSpec) GetImageWithDefault(def string) string {
	if s == nil || s.Image == nil {
		return def
	}
	return *s.Image
}

type ImageResolver struct {
	Version             string
	ImageName           string
	DefaultRepo         string
	DefaultRepoOverride *string
	ImageOverride       *ImageSpec
}

func (r ImageResolver) Resolve() (result ImageSpec) {
	// If a custom image is specified, use it.
	if r.ImageOverride != nil {
		if r.ImageOverride.ImagePullPolicy != nil {
			result.ImagePullPolicy = r.ImageOverride.ImagePullPolicy
		}
		if len(r.ImageOverride.ImagePullSecrets) > 0 {
			result.ImagePullSecrets = r.ImageOverride.ImagePullSecrets
		}
		if r.ImageOverride.Image != nil {
			// If image is set, nothing else needs to be done
			result.Image = r.ImageOverride.Image
			return
		}
	}

	// If a different image repo is requested, use that with the default image
	// name and version tag.
	defaultRepo := r.DefaultRepo
	if r.DefaultRepoOverride != nil {
		defaultRepo = *r.DefaultRepoOverride
	}
	version := r.Version
	if r.Version == "" {
		version = "latest"
	}
	result.Image = lo.ToPtr(fmt.Sprintf("%s:%s",
		path.Join(defaultRepo, r.ImageName), version))
	return
}

type PrometheusReference struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

type PersistenceSpec struct {
	Enabled          bool                                `json:"enabled,omitempty"`
	StorageClassName *string                             `json:"storageClass,omitempty"`
	AccessModes      []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
	// Storage size request. Defaults to 10Gi.
	Request resource.Quantity `json:"request,omitempty"`
}

type ExtraVolumeMount struct {
	Name                string `json:"name,omitempty"`
	MountPath           string `json:"mountPath,omitempty"`
	ReadOnly            bool   `json:"readOnly,omitempty"`
	corev1.VolumeSource `json:",inline"`
}

type OpensearchClusterRef struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

type NeuralSearchSettings struct {
	Enabled   bool   `json:"enabled,omitempty"`
	CustomUrl string `json:"url,omitempty"`
}

func (o *OpensearchClusterRef) ObjectKeyFromRef() types.NamespacedName {
	return types.NamespacedName{
		Name:      o.Name,
		Namespace: o.Namespace,
	}
}
