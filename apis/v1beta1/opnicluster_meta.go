package v1beta1

import (
	"fmt"
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
)

type ServiceKind int

const (
	InferenceService ServiceKind = iota
	DrainService
	PreprocessingService
	PayloadReceiverService
)

func (s ServiceKind) String() string {
	switch s {
	case InferenceService:
		return "inference"
	case DrainService:
		return "drain"
	case PreprocessingService:
		return "preprocessing"
	case PayloadReceiverService:
		return "payload-receiver"
	default:
		return ""
	}
}

func (s ServiceKind) ServiceName() string {
	return "opni-" + s.String()
}

func (s ServiceKind) ImageName() string {
	return s.ServiceName() + "-service"
}

func (s ServiceKind) GetImageSpec(opniCluster *OpniCluster) *ImageSpec {
	switch s {
	case InferenceService:
		return &opniCluster.Spec.Services.Inference.ImageSpec
	case DrainService:
		return &opniCluster.Spec.Services.Drain.ImageSpec
	case PreprocessingService:
		return &opniCluster.Spec.Services.Preprocessing.ImageSpec
	case PayloadReceiverService:
		return &opniCluster.Spec.Services.PayloadReceiver.ImageSpec
	default:
		return nil
	}
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
