package v1beta2

import (
	"time"

	grafanav1alpha1 "github.com/grafana-operator/grafana-operator/v4/api/integreatly/v1alpha1"
	"github.com/rancher/opni/pkg/auth/openid"
	"github.com/rancher/opni/pkg/config/v1beta1"
	cfgv1beta1 "github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/noauth"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ImageSpec struct {
	Image            *string                       `json:"image,omitempty"`
	ImagePullPolicy  *corev1.PullPolicy            `json:"imagePullPolicy,omitempty"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

func (i *ImageSpec) GetImageWithDefault(def string) string {
	if i == nil || i.Image == nil {
		return def
	}
	return *i.Image
}

func (i *ImageSpec) GetImagePullPolicy() corev1.PullPolicy {
	if i == nil || i.ImagePullPolicy == nil {
		return corev1.PullPolicy("")
	}
	return *i.ImagePullPolicy
}

type GatewaySpec struct {
	Image *ImageSpec `json:"image,omitempty"`
	//+kubebuilder:validation:Required
	Auth             AuthSpec `json:"auth,omitempty"`
	Hostname         string   `json:"hostname,omitempty"`
	PluginSearchDirs []string `json:"pluginSearchDirs,omitempty"`
	//+kubebuilder:default=LoadBalancer
	ServiceType        corev1.ServiceType     `json:"serviceType,omitempty"`
	ServiceAnnotations map[string]string      `json:"serviceAnnotations,omitempty"`
	Management         v1beta1.ManagementSpec `json:"management,omitempty"`
	//+kubebuilder:default=etcd
	StorageType cfgv1beta1.StorageType `json:"storageType,omitempty"`

	NodeSelector      map[string]string           `json:"nodeSelector,omitempty"`
	Tolerations       []corev1.Toleration         `json:"tolerations,omitempty"`
	Affinity          *corev1.Affinity            `json:"affinity,omitempty"`
	ExtraVolumeMounts []opnimeta.ExtraVolumeMount `json:"extraVolumeMounts,omitempty"`
	ExtraEnvVars      []corev1.EnvVar             `json:"extraEnvVars,omitempty"`
}

func (g *GatewaySpec) GetServiceType() corev1.ServiceType {
	if g == nil || g.ServiceType == corev1.ServiceType("") {
		return corev1.ServiceTypeClusterIP
	}
	return g.ServiceType
}

type AuthSpec struct {
	//+kubebuilder:validation:Required
	Provider cfgv1beta1.AuthProviderType `json:"provider,omitempty"`
	Openid   *OpenIDConfigSpec           `json:"openid,omitempty"`
	Noauth   *noauth.ServerConfig        `json:"noauth,omitempty"`
}

type OpenIDConfigSpec struct {
	openid.OpenidConfig `json:",inline,omitempty,squash"`
	ClientID            string   `json:"clientID,omitempty"`
	ClientSecret        string   `json:"clientSecret,omitempty"`
	Scopes              []string `json:"scopes,omitempty"`
	AllowedDomains      []string `json:"allowedDomains,omitempty"`
	RoleAttributePath   string   `json:"roleAttributePath,omitempty"`

	InsecureSkipVerify *bool `json:"insecureSkipVerify,omitempty"`

	// extra options from grafana config
	AllowSignUp         *bool  `json:"allowSignUp,omitempty"`
	RoleAttributeStrict *bool  `json:"roleAttributeStrict,omitempty"`
	EmailAttributePath  string `json:"emailAttributePath,omitempty"`
	TLSClientCert       string `json:"tlsClientCert,omitempty"`
	TLSClientKey        string `json:"tlsClientKey,omitempty"`
	TLSClientCA         string `json:"tlsClientCA,omitempty"`
}

type StorageBackendType string

const (
	StorageBackendS3         StorageBackendType = "s3"
	StorageBackendGCS        StorageBackendType = "gcs"
	StorageBackendAzure      StorageBackendType = "azure"
	StorageBackendSwift      StorageBackendType = "swift"
	StorageBackendFilesystem StorageBackendType = "filesystem"
)

type CortexSpec struct {
	Enabled      bool              `json:"enabled,omitempty"`
	Image        *ImageSpec        `json:"image,omitempty"`
	LogLevel     string            `json:"logLevel,omitempty"`
	Storage      CortexStorageSpec `json:"storage,omitempty"`
	ExtraEnvVars []corev1.EnvVar   `json:"extraEnvVars,omitempty"`
}

type CortexStorageSpec struct {
	Backend    StorageBackendType     `json:"backend,omitempty"`
	S3         *S3StorageSpec         `json:"s3,omitempty"`
	GCS        *GCSStorageSpec        `json:"gcs,omitempty"`
	Azure      *AzureStorageSpec      `json:"azure,omitempty"`
	Swift      *SwiftStorageSpec      `json:"swift,omitempty"`
	Filesystem *FilesystemStorageSpec `json:"filesystem,omitempty"`
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
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
type MonitoringCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MonitoringClusterSpec   `json:"spec,omitempty"`
	Status            MonitoringClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
type MonitoringClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MonitoringCluster `json:"items"`
}

type GatewayStatus struct {
	Image           string                      `json:"image,omitempty"`
	ImagePullPolicy corev1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	ServiceName     string                      `json:"serviceName,omitempty"`
	LoadBalancer    *corev1.LoadBalancerIngress `json:"loadBalancer,omitempty"`
	Endpoints       []corev1.EndpointAddress    `json:"endpoints,omitempty"`
	Ready           bool                        `json:"ready,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
type Gateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              GatewaySpec   `json:"spec,omitempty"`
	Status            GatewayStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
type GatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Gateway `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&MonitoringCluster{}, &MonitoringClusterList{},
		&Gateway{}, &GatewayList{},
	)
}
