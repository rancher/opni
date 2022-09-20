package v1beta1

import (
	"time"

	grafanav1alpha1 "github.com/grafana-operator/grafana-operator/v4/api/integreatly/v1alpha1"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AlertingSpec struct {
	Enabled bool `json:"enabled,omitempty"`
	//+kubebuilder:default=9093
	WebPort int `json:"webPort,omitempty"`
	//+kubebuilder:default=9094
	ApiPort int `json:"apiPort,omitempty"`
	//+kubebuilder:default=ClusterIP
	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`
	//+kubebuilder:default="500Mi"
	Storage string `json:"storage,omitempty"`
	//+kubebuilder:default="alertmanager-config"
	ConfigName          string                      `json:"configName,omitempty"`
	GatewayVolumeMounts []opnimeta.ExtraVolumeMount `json:"alertVolumeMounts,omitempty"`
}

type StorageBackendType string

const (
	StorageBackendS3         StorageBackendType = "s3"
	StorageBackendGCS        StorageBackendType = "gcs"
	StorageBackendAzure      StorageBackendType = "azure"
	StorageBackendSwift      StorageBackendType = "swift"
	StorageBackendFilesystem StorageBackendType = "filesystem"
)

type DeploymentMode string

const (
	DeploymentModeAllInOne        DeploymentMode = "AllInOne"
	DeploymentModeHighlyAvailable DeploymentMode = "HighlyAvailable"
)

type CortexSpec struct {
	Enabled        bool                `json:"enabled,omitempty"`
	Image          *opnimeta.ImageSpec `json:"image,omitempty"`
	LogLevel       string              `json:"logLevel,omitempty"`
	Storage        CortexStorageSpec   `json:"storage,omitempty"`
	ExtraEnvVars   []corev1.EnvVar     `json:"extraEnvVars,omitempty"`
	DeploymentMode DeploymentMode      `json:"deploymentMode,omitempty"`

	// Overrides for specific workloads. If unset, all values have automatic
	// defaults based on the deployment mode.
	Workloads CortexWorkloadsSpec `json:"workloads,omitempty"`
}

type CortexStorageSpec struct {
	Backend      StorageBackendType                                      `json:"backend,omitempty"`
	S3           *S3StorageSpec                                          `json:"s3,omitempty"`
	GCS          *GCSStorageSpec                                         `json:"gcs,omitempty"`
	Azure        *AzureStorageSpec                                       `json:"azure,omitempty"`
	Swift        *SwiftStorageSpec                                       `json:"swift,omitempty"`
	Filesystem   *FilesystemStorageSpec                                  `json:"filesystem,omitempty"`
	PVCRetention *appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy `json:"pvcRetention,omitempty"`
}

type CortexWorkloadsSpec struct {
	Distributor   *CortexWorkloadSpec `json:"distributor,omitempty"`
	Ingester      *CortexWorkloadSpec `json:"ingester,omitempty"`
	Compactor     *CortexWorkloadSpec `json:"compactor,omitempty"`
	StoreGateway  *CortexWorkloadSpec `json:"storeGateway,omitempty"`
	Ruler         *CortexWorkloadSpec `json:"ruler,omitempty"`
	QueryFrontend *CortexWorkloadSpec `json:"queryFrontend,omitempty"`
	Querier       *CortexWorkloadSpec `json:"querier,omitempty"`
	Purger        *CortexWorkloadSpec `json:"purger,omitempty"`

	// Used only when deploymentMode is AllInOne.
	AllInOne *CortexWorkloadSpec `json:"allInOne,omitempty"`
}

type CortexWorkloadSpec struct {
	Replicas           *int32                            `json:"replicas,omitempty"`
	ExtraVolumes       []corev1.Volume                   `json:"extraVolumes,omitempty"`
	ExtraVolumeMounts  []corev1.VolumeMount              `json:"extraVolumeMounts,omitempty"`
	ExtraEnvVars       []corev1.EnvVar                   `json:"extraEnvVars,omitempty"`
	ExtraArgs          []string                          `json:"extraArgs,omitempty"`
	SidecarContainers  []corev1.Container                `json:"sidecarContainers,omitempty"`
	InitContainers     []corev1.Container                `json:"initContainers,omitempty"`
	StorageSize        *string                           `json:"storageSize,omitempty"`
	DeploymentStrategy *appsv1.DeploymentStrategy        `json:"deploymentStrategy,omitempty"`
	UpdateStrategy     *appsv1.StatefulSetUpdateStrategy `json:"updateStrategy,omitempty"`
	SecurityContext    *corev1.SecurityContext           `json:"securityContext,omitempty"`
	Affinity           *corev1.Affinity                  `json:"affinity,omitempty"`
}

type S3StorageSpec struct {
	// The S3 bucket endpoint. It could be an AWS S3 endpoint listed at
	// https://docs.aws.amazon.com/general/latest/gr/s3.html or the address of an
	// S3-compatible service in hostname:port format.
	Endpoint string `json:"endpoint,omitempty"`
	// S3 region. If unset, the client will issue a S3 GetBucketLocation API call
	// to autodetect it.
	Region string `json:"region,omitempty"`
	// S3 bucket name
	BucketName string `json:"bucketName,omitempty"`
	// S3 secret access key
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
	// S3 access key ID
	AccessKeyID string `json:"accessKeyID,omitempty"`
	// If enabled, use http:// for the S3 endpoint instead of https://. This could
	// be useful in local dev/test environments while using an S3-compatible
	// backend storage, like Minio.
	Insecure bool `json:"insecure,omitempty"`
	// The signature version to use for authenticating against S3.
	// Supported values are: v4, v2
	SignatureVersion string `json:"signatureVersion,omitempty"`

	SSE  SSEConfig  `json:"sse,omitempty"`
	HTTP HTTPConfig `json:"http,omitempty"`
}

type SSEConfig struct {
	// Enable AWS Server Side Encryption. Supported values: SSE-KMS, SSE-S3
	Type string `json:"typ,omitempty"`
	// KMS Key ID used to encrypt objects in S3
	KMSKeyID string `json:"kmsKeyID,omitempty"`
	// KMS Encryption Context used for object encryption. It expects a JSON formatted string.
	KMSEncryptionContext string `json:"kmsEncryptionContext,omitempty"`
}

type HTTPConfig struct {
	// The time an idle connection will remain idle before closing.
	IdleConnTimeout time.Duration `json:"idleConnTimeout,omitempty"`
	// The amount of time the client will wait for a servers response headers.
	ResponseHeaderTimeout time.Duration `json:"responseHeaderTimeout,omitempty"`
	// If the client connects via HTTPS and this option is enabled, the client will accept any certificate and hostname.
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
	// Maximum time to wait for a TLS handshake. 0 means no limit.
	TLSHandshakeTimeout time.Duration `json:"tlsHandshakeTimeout,omitempty"`
	// The time to wait for a server's first response headers after fully writing the request headers if the request has an Expect header. 0 to send the request body immediately.
	ExpectContinueTimeout time.Duration `json:"expectContinueTimeout,omitempty"`
	// Maximum number of idle (keep-alive) connections across all hosts. 0 means no limit.
	MaxIdleConns int `json:"maxIdleConnections,omitempty"`
	// Maximum number of idle (keep-alive) connections to keep per-host. If 0, a built-in default value is used.
	MaxIdleConnsPerHost int `json:"maxIdleConnectionsPerHost,omitempty"`
	// Maximum number of connections per host. 0 means no limit.
	MaxConnsPerHost int `json:"maxConnectionsPerHost,omitempty"`
}

type GCSStorageSpec struct {
	// GCS bucket name
	BucketName string `json:"bucketName,omitempty"`
	// JSON representing either a Google Developers Console client_credentials.json file
	// or a Google Developers service account key file. If empty, fallback to Google default logic.
	ServiceAccount string `json:"serviceAccount,omitempty"`
}

type AzureStorageSpec struct {
	// Azure storage account name
	StorageAccountName string `json:"accountName,omitempty"`
	// Azure storage account key
	StorageAccountKey string `json:"accountKey,omitempty"`
	// Azure storage container name
	ContainerName string `json:"containerName,omitempty"`
	// Azure storage endpoint suffix without schema. The account name will be
	// prefixed to this value to create the FQDN
	Endpoint string `json:"endpointSuffix,omitempty"`
	// Number of retries for recoverable errors
	MaxRetries int `json:"maxRetries,omitempty"`

	HTTP HTTPConfig `json:"http,omitempty"`
}

type SwiftStorageSpec struct {
	// OpenStack Swift authentication API version. 0 to autodetect.
	AuthVersion int `json:"authVersion,omitempty"`
	// OpenStack Swift authentication URL.
	AuthURL string `json:"authUrl,omitempty"`
	// OpenStack Swift username.
	Username string `json:"username,omitempty"`
	// OpenStack Swift user's domain name.
	UserDomainName string `json:"userDomainName,omitempty"`
	// OpenStack Swift user's domain ID.
	UserDomainID string `json:"userDomainID,omitempty"`
	// OpenStack Swift user ID.
	UserID string `json:"userID,omitempty"`
	// OpenStack Swift API key.
	Password string `json:"password,omitempty"`
	// OpenStack Swift user's domain ID.
	DomainID string `json:"domainID,omitempty"`
	// OpenStack Swift user's domain name.
	DomainName string `json:"domainName,omitempty"`
	// OpenStack Swift project ID (v2,v3 auth only).
	ProjectID string `json:"projectID,omitempty"`
	// OpenStack Swift project name (v2,v3 auth only).
	ProjectName string `json:"projectName,omitempty"`
	// ID of the OpenStack Swift project's domain (v3 auth only), only needed
	// if it differs the from user domain.
	ProjectDomainID string `json:"projectDomainID,omitempty"`
	// Name of the OpenStack Swift project's domain (v3 auth only), only needed
	// if it differs from the user domain.
	ProjectDomainName string `json:"projectDomainName,omitempty"`
	// OpenStack Swift Region to use (v2,v3 auth only).
	RegionName string `json:"regionName,omitempty"`
	// Name of the OpenStack Swift container to use. The container must already
	// exist.
	ContainerName string `json:"containerName,omitempty"`
	// Max number of times to retry failed requests.
	MaxRetries int `json:"maxRetries,omitempty"`
	// Time after which a connection attempt is aborted.
	ConnectTimeout time.Duration `json:"connectTimeout,omitempty"`
	// Time after which an idle request is aborted. The timeout watchdog is reset
	// each time some data is received, so the timeout triggers after X time no
	// data is received on a request.
	RequestTimeout time.Duration `json:"requestTimeout,omitempty"`
}

type FilesystemStorageSpec struct {
	// Local filesystem storage directory.
	Directory string `json:"directory,omitempty"`
}

type GrafanaSpec struct {
	Enabled bool `json:"enabled,omitempty"`
	//+kubebuilder:validation:Required
	Hostname string `json:"hostname"`

	// Contains any additional configuration or overrides for the Grafana
	// installation spec.
	grafanav1alpha1.GrafanaSpec `json:",inline,omitempty"`
}

type MonitoringClusterSpec struct {
	//+kubebuilder:validation:Required
	Gateway corev1.LocalObjectReference `json:"gateway,omitempty"`
	Cortex  CortexSpec                  `json:"cortex,omitempty"`
	Grafana GrafanaSpec                 `json:"grafana,omitempty"`
}

type MonitoringClusterStatus struct {
	Image           string            `json:"image,omitempty"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	Cortex          CortexStatus      `json:"cortex,omitempty"`
}

type CortexStatus struct {
	Version string `json:"version,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type MonitoringCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MonitoringClusterSpec   `json:"spec,omitempty"`
	Status            MonitoringClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type MonitoringClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MonitoringCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&MonitoringCluster{}, &MonitoringClusterList{},
	)
}
