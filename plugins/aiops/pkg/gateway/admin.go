package gateway

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"slices"

	"github.com/Masterminds/semver"
	aiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/util/nats"
	"github.com/rancher/opni/pkg/versions"
	"github.com/rancher/opni/plugins/aiops/apis/admin"
	"github.com/samber/lo"
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
	OpniS3SecretName = "opni-s3-credentials"
	AccessKeyKey     = "accessKey"
	SecretKeyKey     = "secretKey"
)

var (
	DefaultModelSources = map[pretrainedModelType]*string{
		pretrainedModelControlplane: lo.ToPtr("https://opni-pretrained-models.s3.us-east-2.amazonaws.com/control-plane-model-v0.4.2.zip"),
		pretrainedModelRancher:      lo.ToPtr("https://opni-pretrained-models.s3.us-east-2.amazonaws.com/rancher-model-v0.4.2.zip"),
		pretrainedModelLonghorn:     lo.ToPtr("https://opni-pretrained-models.s3.us-east-2.amazonaws.com/longhorn-model-v0.6.0.zip"),
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

func (s *AIOpsPlugin) getPretrainedModel(ctx context.Context, modelType pretrainedModelType) (*admin.PretrainedModel, error) {
	_, ok := DefaultModelSources[modelType]
	if !ok {
		err := status.Error(codes.InvalidArgument, "invalid model type")
		return nil, err
	}
	model := &aiv1beta1.PretrainedModel{}
	err := s.k8sClient.Get(ctx, types.NamespacedName{
		Name:      modelObjectName(modelType),
		Namespace: s.storageNamespace,
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

func (s *AIOpsPlugin) putPretrainedModel(
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
			updateModelSpec(modelType, model, modelObject)
			return s.k8sClient.Update(ctx, modelObject)
		})
		return &emptypb.Empty{}, err
	}

	updateModelSpec(modelType, model, modelObject)
	return &emptypb.Empty{}, s.k8sClient.Create(ctx, modelObject)
}

func (s *AIOpsPlugin) GetAISettings(ctx context.Context, _ *emptypb.Empty) (*admin.AISettings, error) {
	opni := &aiv1beta1.OpniCluster{}
	err := s.k8sClient.Get(ctx, types.NamespacedName{
		Name:      OpniServicesName,
		Namespace: s.storageNamespace,
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

	var s3Settings *admin.S3Settings
	if opni.Spec.S3.External != nil {
		secret := &corev1.Secret{}
		err := s.k8sClient.Get(ctx, types.NamespacedName{
			Name:      opni.Spec.S3.External.Credentials.Name,
			Namespace: opni.Spec.S3.External.Credentials.Namespace,
		}, secret)
		if err != nil {
			s.Logger.Error("failed to get s3 secret", logger.Err(err))
		}
		s3Settings = &admin.S3Settings{
			Endpoint:    opni.Spec.S3.External.Endpoint,
			NulogBucket: &opni.Spec.S3.NulogS3Bucket,
			DrainBucket: &opni.Spec.S3.DrainS3Bucket,
			AccessKey:   string(secret.Data["accessKey"]),
		}
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
		S3Settings: s3Settings,
	}, nil
}

func (s *AIOpsPlugin) PutAISettings(ctx context.Context, settings *admin.AISettings) (*emptypb.Empty, error) {
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

	version := s.version
	if versions.Version != "" && versions.Version != "unversioned" {
		version = versions.Version
	}

	err := s.createOrUpdateS3Secret(ctx, settings.GetS3Settings())
	if err != nil {
		return nil, err
	}

	exists := true
	opniCluster := &aiv1beta1.OpniCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OpniServicesName,
			Namespace: s.storageNamespace,
		},
		Spec: aiv1beta1.OpniClusterSpec{
			NatsRef: corev1.LocalObjectReference{
				Name: nats.NatsObjectNameFromURL(os.Getenv("NATS_SERVER_URL")),
			},
			Services: aiv1beta1.ServicesSpec{
				Metrics: aiv1beta1.MetricsServiceSpec{
					Enabled: lo.ToPtr(false),
				},
				PayloadReceiver: aiv1beta1.PayloadReceiverServiceSpec{
					Enabled: lo.ToPtr(false),
				},
			},
			Opensearch: s.opensearchCluster,
		},
	}
	err = s.k8sClient.Get(ctx, client.ObjectKeyFromObject(opniCluster), opniCluster)
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
		s3SettingsMutator(settings.GetS3Settings(), opniCluster)

		return &emptypb.Empty{}, s.k8sClient.Create(ctx, opniCluster)
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := s.k8sClient.Get(ctx, client.ObjectKeyFromObject(opniCluster), opniCluster)
		if err != nil {
			return err
		}
		opniCluster.Spec.Services.Inference.PretrainedModels = models
		gpuSettingsMutator(settings.GetGpuSettings(), opniCluster)
		s3SettingsMutator(settings.GetS3Settings(), opniCluster)
		return s.k8sClient.Update(ctx, opniCluster)
	})
	return &emptypb.Empty{}, err
}

func (s *AIOpsPlugin) DeleteAISettings(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	err := s.k8sClient.Delete(ctx, &aiv1beta1.OpniCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OpniServicesName,
			Namespace: s.storageNamespace,
		},
	})
	if err != nil {
		return nil, err
	}

	err = s.k8sClient.Delete(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OpniS3SecretName,
			Namespace: s.storageNamespace,
		},
	})
	if client.IgnoreNotFound(err) != nil {
		return nil, err
	}

	err = s.k8sClient.DeleteAllOf(ctx, &aiv1beta1.PretrainedModel{}, client.InNamespace(s.storageNamespace))
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *AIOpsPlugin) UpgradeAvailable(ctx context.Context, _ *emptypb.Empty) (*admin.UpgradeAvailableResponse, error) {
	version := s.version
	if versions.Version != "" && versions.Version != "unversioned" {
		version = versions.Version
	}
	newVersion, err := semver.NewVersion(version)
	if err != nil {
		return nil, err
	}
	opniCluster := &aiv1beta1.OpniCluster{}
	err = s.k8sClient.Get(ctx, types.NamespacedName{
		Name:      OpniServicesName,
		Namespace: s.storageNamespace,
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

func (s *AIOpsPlugin) DoUpgrade(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	version := s.version
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
			Namespace: s.storageNamespace,
		},
	}
	err = s.k8sClient.Get(ctx, client.ObjectKeyFromObject(opniCluster), opniCluster)
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
		err := s.k8sClient.Get(ctx, client.ObjectKeyFromObject(opniCluster), opniCluster)
		if err != nil {
			return err
		}
		opniCluster.Spec.Version = "v" + strings.TrimPrefix(version, "v")
		return s.k8sClient.Update(ctx, opniCluster)
	})
	return &emptypb.Empty{}, err
}

func (s *AIOpsPlugin) GetRuntimeClasses(ctx context.Context, _ *emptypb.Empty) (*admin.RuntimeClassResponse, error) {
	runtimeClasses := &nodev1.RuntimeClassList{}
	if err := s.k8sClient.List(ctx, runtimeClasses); err != nil {
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

func (s *AIOpsPlugin) createOrUpdateS3Secret(ctx context.Context, settings *admin.S3Settings) error {
	if settings == nil {
		return nil
	}
	exists := true
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OpniS3SecretName,
			Namespace: s.storageNamespace,
		},
		StringData: map[string]string{
			AccessKeyKey: settings.AccessKey,
			SecretKeyKey: settings.SecretKey,
		},
	}
	err := s.k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return err
		}
		exists = false
	}

	if !exists {
		return s.k8sClient.Create(ctx, secret)
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := s.k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
		if err != nil {
			return err
		}
		secret.Data[AccessKeyKey] = []byte(settings.AccessKey)
		if settings.SecretKey != "" {
			secret.Data[SecretKeyKey] = []byte(settings.SecretKey)
		}
		return s.k8sClient.Update(ctx, secret)
	})

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

func s3SettingsMutator(settings *admin.S3Settings, cluster *aiv1beta1.OpniCluster) {
	if settings == nil {
		cluster.Spec.S3 = aiv1beta1.S3Spec{
			Internal: &aiv1beta1.InternalSpec{},
		}
		return
	}
	endpoint := settings.GetEndpoint()
	if endpoint != "" {
		if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
			endpoint = "https://" + endpoint
		}
	}
	cluster.Spec.S3 = aiv1beta1.S3Spec{
		NulogS3Bucket: settings.GetNulogBucket(),
		DrainS3Bucket: settings.GetDrainBucket(),
		External: &aiv1beta1.ExternalSpec{
			Endpoint: endpoint,
			Credentials: &corev1.SecretReference{
				Name:      OpniS3SecretName,
				Namespace: cluster.Namespace,
			},
		},
	}
}
