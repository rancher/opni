package thanos

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	thanosv1alpha1 "github.com/banzaicloud/thanos-operator/pkg/sdk/api/v1alpha1"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/s3utils"
	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ThanosBucketName = "opni-thanos"
)

type Reconciler struct {
	opniCluster *v1beta1.OpniCluster
	client      client.Client
	ctx         context.Context
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	opniCluster *v1beta1.OpniCluster,
) *Reconciler {
	return &Reconciler{
		client:      client,
		opniCluster: opniCluster,
		ctx:         ctx,
	}
}

// Returns the given endpoint with the scheme removed, and a bool indicating
// whether the scheme denotes an insecure connection.
func formatS3Endpoint(u url.URL) (string, bool, error) {
	return strings.Replace(u.String(), u.Scheme+"://", "", 1), u.Scheme == "http", nil
}

func (r *Reconciler) EnsureThanosBucketExists() error {
	u, err := url.Parse(r.opniCluster.Status.Auth.S3Endpoint)
	if err != nil {
		return err
	}
	endpoint, insecure, err := formatS3Endpoint(*u)
	if err != nil {
		return err
	}
	region := s3utils.GetRegionFromURL(*u)
	client, err := minio.New(endpoint, &minio.Options{
		Creds: credentials.New(&util.S3CredentialsProvider{
			Client:      r.client,
			Namespace:   r.opniCluster.Namespace,
			AccessKey:   r.opniCluster.Status.Auth.S3AccessKey,
			SecretKey:   r.opniCluster.Status.Auth.S3SecretKey,
			ShouldCache: false,
		}),
		Secure:       !insecure,
		Region:       region,
		BucketLookup: minio.BucketLookupAuto,
		Transport:    http.DefaultTransport,
	})
	if err != nil {
		return err
	}
	ctx, ca := context.WithTimeout(r.ctx, 2*time.Second)
	defer ca()
	switch exists, err := client.BucketExists(ctx, ThanosBucketName); {
	case err != nil:
		return err
	case !exists:
		err := client.MakeBucket(ctx, ThanosBucketName, minio.MakeBucketOptions{
			Region: region,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) objectStoreSecret() ([]resources.Resource, error) {
	if r.opniCluster.Spec.S3.Internal == nil && r.opniCluster.Spec.S3.External == nil {
		return nil, nil
	}

	if r.opniCluster.Status.Auth.S3AccessKey == nil ||
		r.opniCluster.Status.Auth.S3SecretKey == nil ||
		r.opniCluster.Status.Auth.S3Endpoint == "" {
		return nil, fmt.Errorf("S3 auth info unavailable")
	}

	credsProvider := util.S3CredentialsProvider{
		Client:      r.client,
		Namespace:   r.opniCluster.Namespace,
		AccessKey:   r.opniCluster.Status.Auth.S3AccessKey,
		SecretKey:   r.opniCluster.Status.Auth.S3SecretKey,
		ShouldCache: false,
	}
	creds, err := credsProvider.Retrieve()
	if err != nil {
		return nil, err
	}

	objectStoreYaml := `
type: S3
config:
  endpoint: %s
  insecure: %s
  bucket: %s
  access_key: %s
  secret_key: %s
`

	u, err := url.Parse(r.opniCluster.Status.Auth.S3Endpoint)
	if err != nil {
		return nil, err
	}

	endpoint, insecure, err := formatS3Endpoint(*u)
	if err != nil {
		return nil, err
	}

	thanosSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-thanos-objectstore",
			Namespace: r.opniCluster.Spec.Services.Metrics.PrometheusNamespace,
		},
		StringData: map[string]string{
			"object-store.yaml": fmt.Sprintf(objectStoreYaml,
				endpoint,
				fmt.Sprint(insecure),
				ThanosBucketName,
				creds.AccessKeyID,
				creds.SecretAccessKey,
			),
		},
	}

	ctrl.SetControllerReference(r.opniCluster, thanosSecret, r.client.Scheme())
	if pointer.BoolDeref(r.opniCluster.Spec.Services.Metrics.Enabled, false) {
		return []resources.Resource{
			resources.Present(thanosSecret),
		}, nil
	}
	return []resources.Resource{
		resources.Absent(thanosSecret),
	}, nil
}

// TODO: add support for replacing the default image
var (
	DefaultThanosImageRepository = thanosv1alpha1.ThanosImageRepository
	DefaultThanosImageTag        = thanosv1alpha1.ThanosImageTag
)

func (r *Reconciler) ThanosResources() (resourceList []resources.Resource, _ error) {
	// Create the object store secret first
	objectStoreSecret, err := r.objectStoreSecret()
	if err != nil {
		return nil, err
	}
	resourceList = append(resourceList, objectStoreSecret...)
	resourceList = append(resourceList, r.thanosWorkloads()...)
	return
}
