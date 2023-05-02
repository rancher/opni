package log_anomaly

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Masterminds/semver"
	aiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
	"github.com/rancher/opni/pkg/util/nats/k8snats"
	"github.com/rancher/opni/pkg/versions"
	"github.com/rancher/opni/plugins/aiops/pkg/apis/admin"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	corev1 "k8s.io/api/core/v1"
	nodev1 "k8s.io/api/node/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type pretrainedModelType string

const (
	pretrainedModelControlplane pretrainedModelType = "controlplane"
	pretrainedModelRancher      pretrainedModelType = "rancher"
	pretrainedModelLonghorn     pretrainedModelType = "longhorn"
)

const (
	OpniServicesName = "opni"
)

var (
	DefaultModelSources = map[pretrainedModelType]*string{
		pretrainedModelControlplane: lo.ToPtr("https://opni-public.s3.us-east-2.amazonaws.com/pretrain-models/control-plane-model-v0.4.2.zip"),
		pretrainedModelRancher:      lo.ToPtr("https://opni-public.s3.us-east-2.amazonaws.com/pretrain-models/rancher-model-v0.4.2.zip"),
		pretrainedModelLonghorn:     lo.ToPtr("https://opni-public.s3.us-east-2.amazonaws.com/pretrain-models/longhorn-model-v0.6.0.zip"),
	}
	ModelHyperParameters = map[pretrainedModelType]map[string]intstr.IntOrString{
		pretrainedModelControlplane: {
			"modelThreshold": intstr.FromString("0.6"),
			"minLogTokens":   intstr.FromInt(1),
			"serviceType":    intstr.FromString("control-plane"),
		},
		pretrainedModelRancher: {
			"modelThreshold": intstr.FromString("0.6"),
			"minLogTokens":   intstr.FromInt(1),
			"serviceType":    intstr.FromString("rancher"),
		},
		pretrainedModelLonghorn: {
			"modelThreshold": intstr.FromString("0.8"),
			"minLogTokens":   intstr.FromInt(1),
			"serviceType":    intstr.FromString("longhorn"),
		},
	}
)

func (s *LogAnomaly) getPretrainedModel(ctx context.Context, modelType pretrainedModelType) (*admin.PretrainedModel, error) {
	_, ok := DefaultModelSources[modelType]
	if !ok {
		err := status.Error(codes.InvalidArgument, "invalid model type")
		return nil, err
	}
	model := &aiv1beta1.PretrainedModel{}
	err := s.K8sClient.Get(ctx, types.NamespacedName{
		Name:      modelObjectName(modelType),
		Namespace: s.StorageNamespace,
	}, model)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return &admin.PretrainedModel{
		HttpSource: func() *string {
			if model.Spec.HTTP == nil {
				return nil
			}
			defaultURL, ok := DefaultModelSources[modelType]
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

func (s *LogAnomaly) putPretrainedModel(
	ctx context.Context,
	modelType pretrainedModelType,
	model *admin.PretrainedModel,
) (*emptypb.Empty, error) {
	_, ok := DefaultModelSources[modelType]
	if !ok {
		err := status.Error(codes.InvalidArgument, "invalid model type")
		return nil, err
	}

	modelObject := &aiv1beta1.PretrainedModel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      modelObjectName(modelType),
			Namespace: s.StorageNamespace,
		},
	}

	exists := true
	err := s.K8sClient.Get(ctx, client.ObjectKeyFromObject(modelObject), modelObject)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil, err
		}
		exists = false
	}

	if exists {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			err := s.K8sClient.Get(ctx, client.ObjectKeyFromObject(modelObject), modelObject)
			if err != nil {
				return err
			}
			updateModelSpec(modelType, model, modelObject)
			return s.K8sClient.Update(ctx, modelObject)
		})
		return &emptypb.Empty{}, err
	}

	updateModelSpec(modelType, model, modelObject)
	return &emptypb.Empty{}, s.K8sClient.Create(ctx, modelObject)
}

func (s *LogAnomaly) GetAISettings(ctx context.Context, _ *emptypb.Empty) (*admin.AISettings, error) {
	opni := &aiv1beta1.OpniCluster{}
	err := s.K8sClient.Get(ctx, types.NamespacedName{
		Name:      OpniServicesName,
		Namespace: s.StorageNamespace,
	}, opni)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			err := status.Error(codes.NotFound, "fetch failed : 404 not found")
			return nil, err
		}
		return nil, err
	}

	controlplane, err := s.getPretrainedModel(ctx, pretrainedModelControlplane)
	if err != nil {
		return nil, err
	}
	rancher, err := s.getPretrainedModel(ctx, pretrainedModelRancher)
	if err != nil {
		return nil, err
	}
	longhorn, err := s.getPretrainedModel(ctx, pretrainedModelLonghorn)
	if err != nil {
		return nil, err
	}

	return &admin.AISettings{
		GpuSettings: func() *admin.GPUSettings {
			if !lo.FromPtrOr(opni.Spec.Services.GPUController.Enabled, true) {
				return nil
			}
			return &admin.GPUSettings{
				RuntimeClass: opni.Spec.Services.GPUController.RuntimeClass,
			}
		}(),
		DrainReplicas: opni.Spec.Services.Drain.Replicas,
		Controlplane: func() *admin.PretrainedModel {
			if modelEnabled(opni.Spec.Services.Inference.PretrainedModels, pretrainedModelControlplane) {
				return controlplane
			}
			return nil
		}(),
		Rancher: func() *admin.PretrainedModel {
			if modelEnabled(opni.Spec.Services.Inference.PretrainedModels, pretrainedModelRancher) {
				return rancher
			}
			return nil
		}(),
		Longhorn: func() *admin.PretrainedModel {
			if modelEnabled(opni.Spec.Services.Inference.PretrainedModels, pretrainedModelLonghorn) {
				return longhorn
			}
			return nil
		}(),
	}, nil
}

func (s *LogAnomaly) PutAISettings(ctx context.Context, settings *admin.AISettings) (*emptypb.Empty, error) {
	models := []corev1.LocalObjectReference{}
	if settings.GetControlplane() != nil {
		models = append(models, corev1.LocalObjectReference{
			Name: modelObjectName(pretrainedModelControlplane),
		})
		_, err := s.putPretrainedModel(ctx, pretrainedModelControlplane, settings.Controlplane)
		if err != nil {
			return nil, err
		}
	}
	if settings.GetRancher() != nil {
		models = append(models, corev1.LocalObjectReference{
			Name: modelObjectName(pretrainedModelRancher),
		})
		_, err := s.putPretrainedModel(ctx, pretrainedModelRancher, settings.Rancher)
		if err != nil {
			return nil, err
		}
	}
	if settings.GetLonghorn() != nil {
		models = append(models, corev1.LocalObjectReference{
			Name: modelObjectName(pretrainedModelLonghorn),
		})
		_, err := s.putPretrainedModel(ctx, pretrainedModelLonghorn, settings.Longhorn)
		if err != nil {
			return nil, err
		}
	}

	version := s.Version
	if versions.Version != "" && versions.Version != "unversioned" {
		version = versions.Version
	}

	exists := true
	opniCluster := &aiv1beta1.OpniCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OpniServicesName,
			Namespace: s.StorageNamespace,
		},
		Spec: aiv1beta1.OpniClusterSpec{
			NatsRef: corev1.LocalObjectReference{
				Name: k8snats.NatsObjectNameFromURL(os.Getenv("NATS_SERVER_URL")),
			},
			Services: aiv1beta1.ServicesSpec{
				Metrics: aiv1beta1.MetricsServiceSpec{
					Enabled: lo.ToPtr(false),
				},
				PayloadReceiver: aiv1beta1.PayloadReceiverServiceSpec{
					Enabled: lo.ToPtr(false),
				},
			},
			Opensearch: s.OpensearchCluster,
			S3: aiv1beta1.S3Spec{
				Internal: &aiv1beta1.InternalSpec{},
			},
		},
	}
	err := s.K8sClient.Get(ctx, client.ObjectKeyFromObject(opniCluster), opniCluster)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil, err
		}
		exists = false
	}

	if !exists {
		opniCluster.Spec.Version = "v" + strings.TrimPrefix(version, "v")
		opniCluster.Spec.Services.Inference.PretrainedModels = models
		gpuSettingsMutator(settings.GetGpuSettings(), opniCluster)

		return &emptypb.Empty{}, s.K8sClient.Create(ctx, opniCluster)
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := s.K8sClient.Get(ctx, client.ObjectKeyFromObject(opniCluster), opniCluster)
		if err != nil {
			return err
		}
		opniCluster.Spec.Services.Inference.PretrainedModels = models
		gpuSettingsMutator(settings.GetGpuSettings(), opniCluster)
		return s.K8sClient.Update(ctx, opniCluster)
	})
	return &emptypb.Empty{}, err
}

func (s *LogAnomaly) DeleteAISettings(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	err := s.K8sClient.Delete(ctx, &aiv1beta1.OpniCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OpniServicesName,
			Namespace: s.StorageNamespace,
		},
	})
	if err != nil {
		return nil, err
	}
	err = s.K8sClient.DeleteAllOf(ctx, &aiv1beta1.PretrainedModel{}, client.InNamespace(s.StorageNamespace))
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *LogAnomaly) UpgradeAvailable(ctx context.Context, _ *emptypb.Empty) (*admin.UpgradeAvailableResponse, error) {
	version := s.Version
	if versions.Version != "" && versions.Version != "unversioned" {
		version = versions.Version
	}
	newVersion, err := semver.NewVersion(version)
	if err != nil {
		return nil, err
	}
	opniCluster := &aiv1beta1.OpniCluster{}
	err = s.K8sClient.Get(ctx, types.NamespacedName{
		Name:      OpniServicesName,
		Namespace: s.StorageNamespace,
	}, opniCluster)
	if err != nil {
		return nil, err
	}

	oldVersion, err := semver.NewVersion(opniCluster.Spec.Version)
	if err != nil {
		return nil, err
	}

	return &admin.UpgradeAvailableResponse{
		UpgradePending: lo.ToPtr(newVersion.GreaterThan(oldVersion)),
	}, nil
}

func (s *LogAnomaly) DoUpgrade(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	version := s.Version
	if versions.Version != "" && versions.Version != "unversioned" {
		version = versions.Version
	}
	newVersion, err := semver.NewVersion(version)
	if err != nil {
		return nil, err
	}

	opniCluster := &aiv1beta1.OpniCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OpniServicesName,
			Namespace: s.StorageNamespace,
		},
	}
	err = s.K8sClient.Get(ctx, client.ObjectKeyFromObject(opniCluster), opniCluster)
	if err != nil {
		return nil, err
	}

	oldVersion, err := semver.NewVersion(opniCluster.Spec.Version)
	if err != nil {
		return nil, err
	}

	if newVersion.LessThan(oldVersion) {
		return nil, errors.New("new version is older, can't upgrade")
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := s.K8sClient.Get(ctx, client.ObjectKeyFromObject(opniCluster), opniCluster)
		if err != nil {
			return err
		}
		opniCluster.Spec.Version = "v" + strings.TrimPrefix(version, "v")
		return s.K8sClient.Update(ctx, opniCluster)
	})
	return &emptypb.Empty{}, err
}

func (s *LogAnomaly) GetRuntimeClasses(ctx context.Context, _ *emptypb.Empty) (*admin.RuntimeClassResponse, error) {
	runtimeClasses := &nodev1.RuntimeClassList{}
	if err := s.K8sClient.List(ctx, runtimeClasses); err != nil {
		return nil, err
	}

	runtimeClassNames := make([]string, 0, len(runtimeClasses.Items))
	for _, runtimeClass := range runtimeClasses.Items {
		runtimeClassNames = append(runtimeClassNames, runtimeClass.Name)
	}

	return &admin.RuntimeClassResponse{
		RuntimeClasses: runtimeClassNames,
	}, nil
}

func updateModelSpec(modelType pretrainedModelType, modelRequest *admin.PretrainedModel, modelObject *aiv1beta1.PretrainedModel) {
	modelObject.Spec = aiv1beta1.PretrainedModelSpec{
		Hyperparameters: ModelHyperParameters[modelType],
		Replicas:        modelRequest.Replicas,
	}

	if modelRequest.HttpSource == nil && modelRequest.ImageSource == nil {
		modelObject.Spec.ModelSource = aiv1beta1.ModelSource{
			HTTP: &aiv1beta1.HTTPSource{
				URL: *DefaultModelSources[modelType],
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

func modelObjectName(modelType pretrainedModelType) string {
	return fmt.Sprintf("opni-model-%s", modelType)
}

func modelEnabled(models []corev1.LocalObjectReference, modelType pretrainedModelType) bool {
	return slices.Contains(models, corev1.LocalObjectReference{
		Name: modelObjectName(modelType),
	})
}

func gpuSettingsMutator(settings *admin.GPUSettings, cluster *aiv1beta1.OpniCluster) {
	cluster.Spec.Services.GPUController.Enabled = lo.ToPtr(settings != nil)
	cluster.Spec.Services.GPUController.RuntimeClass = func() *string {
		if settings == nil {
			return nil
		}
		return settings.RuntimeClass
	}()
	cluster.Spec.Services.TrainingController.Enabled = lo.ToPtr(settings != nil)
	cluster.Spec.Services.Inference.Enabled = lo.ToPtr(settings != nil)
	cluster.Spec.Services.Drain.Workload.Enabled = lo.ToPtr(settings != nil)
}
