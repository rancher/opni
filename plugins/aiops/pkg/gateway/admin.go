package gateway

import (
	"context"
	"fmt"

	"github.com/gogo/status"
	aiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
	"github.com/rancher/opni/plugins/aiops/pkg/apis/admin"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	opniServicesName = "opni"
)

var (
	DefaultModelSources = map[string]*string{
		"controlplane": lo.ToPtr("https://opni-public.s3.us-east-2.amazonaws.com/pretrain-models/control-plane-model-v0.4.2.zip"),
		"rancher":      lo.ToPtr("https://opni-public.s3.us-east-2.amazonaws.com/pretrain-models/rancher-model-v0.4.2.zip"),
		"longhorn":     lo.ToPtr("https://opni-public.s3.us-east-2.amazonaws.com/pretrain-models/longhorn-model-v0.6.0.zip"),
	}
	ModelHyperParameters = map[string]map[string]intstr.IntOrString{
		"controlplane": {
			"modelThreshold": intstr.FromString("0.6"),
			"minLogTokens":   intstr.FromInt(1),
			"serviceType":    intstr.FromString("control-plane"),
		},
		"rancher": {
			"modelThreshold": intstr.FromString("0.6"),
			"minLogTokens":   intstr.FromInt(1),
			"serviceType":    intstr.FromString("rancher"),
		},
		"longhorn": {
			"modelThreshold": intstr.FromString("0.8"),
			"minLogTokens":   intstr.FromInt(1),
			"serviceType":    intstr.FromString("longhorn"),
		},
	}
)

func (s *AIOpsPlugin) GetPretrainedModel(ctx context.Context, modelRef *admin.PretrainedModelRef) (*admin.PretrainedModel, error) {
	model := &aiv1beta1.PretrainedModel{}
	err := s.k8sClient.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("opni-model-%s", modelRef.Type),
		Namespace: s.storageNamespace,
	}, model)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			err := status.Error(codes.NotFound, "fetch failed : 404 not found")
			return nil, err
		}
		return nil, err
	}

	return &admin.PretrainedModel{
		Ref: modelRef,
		HttpSource: func() *string {
			if model.Spec.HTTP == nil {
				return nil
			}
			defaultURL, ok := DefaultModelSources[modelRef.Type]
			if ok && *defaultURL == model.Spec.HTTP.URL {
				return nil
			}
			return &model.Spec.HTTP.URL
		}(),
		ImageSource: func() *string {
			if model.Spec.Container == nil {
				return nil
			}
			return &model.Spec.Container.Image
		}(),
		Replicas: model.Spec.Replicas,
	}, nil
}

func (s *AIOpsPlugin) PutPretrainedModel(ctx context.Context, model *admin.PretrainedModel) (*emptypb.Empty, error) {
	_, ok := DefaultModelSources[model.Ref.Type]
	if !ok {
		err := status.Error(codes.InvalidArgument, "invalid model type")
		return nil, err
	}

	modelObject := &aiv1beta1.PretrainedModel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("opni-model-%s", model.Ref.Type),
			Namespace: s.storageNamespace,
		},
	}

	exists := true
	err := s.k8sClient.Get(ctx, client.ObjectKeyFromObject(modelObject), modelObject)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil, err
		}
		exists = false
	}

	if exists {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			err := s.k8sClient.Get(ctx, client.ObjectKeyFromObject(modelObject), modelObject)
			if err != nil {
				return err
			}
			updateModelSpec(model, modelObject)
			return s.k8sClient.Update(ctx, modelObject)
		})
		return &emptypb.Empty{}, err
	}

	updateModelSpec(model, modelObject)
	return &emptypb.Empty{}, s.k8sClient.Create(ctx, modelObject)
}

func (s *AIOpsPlugin) GetAISettings(ctx context.Context, _ *emptypb.Empty) (*admin.AISettings, error) {
	opni := &aiv1beta1.OpniCluster{}
	err := s.k8sClient.Get(ctx, types.NamespacedName{
		Name:      opniServicesName,
		Namespace: s.storageNamespace,
	}, opni)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			return &admin.AISettings{
				Enabled: false,
			}, nil
		}
		return nil, err
	}

	return &admin.AISettings{
		Enabled:       true,
		DrainReplicas: opni.Spec.Services.Drain.Replicas,
	}, nil
}

func updateModelSpec(modelRequest *admin.PretrainedModel, modelObject *aiv1beta1.PretrainedModel) {
	modelObject.Spec = aiv1beta1.PretrainedModelSpec{
		Hyperparameters: ModelHyperParameters[modelRequest.Ref.Type],
		Replicas:        modelRequest.Replicas,
	}

	if modelRequest.HttpSource == nil && modelRequest.ImageSource == nil {
		modelObject.Spec.ModelSource = aiv1beta1.ModelSource{
			HTTP: &aiv1beta1.HTTPSource{
				URL: *DefaultModelSources[modelRequest.Ref.Type],
			},
		}
	} else {
		if modelRequest.HttpSource != nil {
			modelObject.Spec.ModelSource = aiv1beta1.ModelSource{
				HTTP: &aiv1beta1.HTTPSource{
					URL: *modelRequest.HttpSource,
				},
			}
		}
		if modelRequest.ImageSource != nil {
			modelObject.Spec.ModelSource = aiv1beta1.ModelSource{
				Container: &aiv1beta1.ContainerSource{
					Image: *modelRequest.ImageSource,
				},
			}
		}
	}
}
