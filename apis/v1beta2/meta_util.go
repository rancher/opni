package v1beta2

import (
	"fmt"
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
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
	result.Image = pointer.String(fmt.Sprintf("%s:%s",
		path.Join(defaultRepo, r.ImageName), version))
	return
}
