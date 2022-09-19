package v1

import (
	"fmt"
	"strings"
	"time"

	flag "github.com/spf13/pflag"
	"google.golang.org/protobuf/types/known/durationpb"
)

func (cfg *StorageSpec) FlagSet() *flag.FlagSet {
	cfg.InitEmptyFields()
	fs := flag.NewFlagSet("storage", flag.ExitOnError)
	cfg.RegisterFlagsWithPrefix("storage.", fs)
	return fs
}

func (cfg *StorageSpec) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	cfg.S3.RegisterFlagsWithPrefix(prefix, f)
	cfg.Gcs.RegisterFlagsWithPrefix(prefix, f)
	cfg.Azure.RegisterFlagsWithPrefix(prefix, f)
	cfg.Swift.RegisterFlagsWithPrefix(prefix, f)
	cfg.Filesystem.RegisterFlagsWithPrefix(prefix, f)

	f.StringVar(&cfg.Backend, prefix+"backend", S3, fmt.Sprintf("Backend storage to use. Supported backends are: %s.", strings.Join(supportedBackends, ", ")))
}

type dpb struct {
	*durationpb.Duration
}

func dpbValue(val time.Duration, p **durationpb.Duration) *dpb {
	*p = durationpb.New(val)
	return &dpb{*p}
}

func (d *dpb) Set(s string) error {
	duration, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = dpb{durationpb.New(duration)}
	return nil
}

func (d *dpb) Get() any {
	return d.Duration
}

func (d *dpb) Type() string {
	return "duration"
}

func (d *dpb) String() string {
	return d.Duration.AsDuration().String()
}

func (cfg *S3StorageSpec) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.StringVar(&cfg.AccessKeyID, prefix+"s3.access-key-id", "", "S3 access key ID")
	f.StringVar(&cfg.SecretAccessKey, prefix+"s3.secret-access-key", "", "S3 secret access key")
	f.StringVar(&cfg.BucketName, prefix+"s3.bucket-name", "", "S3 bucket name")
	f.StringVar(&cfg.Region, prefix+"s3.region", "", "S3 region. If unset, the client will issue a S3 GetBucketLocation API call to autodetect it.")
	f.StringVar(&cfg.Endpoint, prefix+"s3.endpoint", "", "The S3 bucket endpoint. It could be an AWS S3 endpoint listed at https://docs.aws.amazon.com/general/latest/gr/s3.html or the address of an S3-compatible service in hostname:port format.")
	f.BoolVar(&cfg.Insecure, prefix+"s3.insecure", false, "If enabled, use http:// for the S3 endpoint instead of https://. This could be useful in local dev/test environments while using an S3-compatible backend storage, like Minio.")
	f.StringVar(&cfg.SignatureVersion, prefix+"s3.signature-version", SignatureVersionV4, fmt.Sprintf("The signature version to use for authenticating against S3. Supported values are: %s.", strings.Join(supportedSignatureVersions, ", ")))
	cfg.Sse.RegisterFlagsWithPrefix(prefix+"s3.sse.", f)
	cfg.Http.RegisterFlagsWithPrefix(prefix, f)
}

func (cfg *GCSStorageSpec) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.StringVar(&cfg.BucketName, prefix+"gcs.bucket-name", "", "GCS bucket name")
	f.StringVar(&cfg.ServiceAccount, prefix+"gcs.service-account", "", "JSON representing either a Google Developers Console client_credentials.json file or a Google Developers service account key file. If empty, fallback to Google default logic.")
}

func (cfg *AzureStorageSpec) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.StringVar(&cfg.StorageAccountName, prefix+"azure.account-name", "", "Azure storage account name")
	f.StringVar(&cfg.StorageAccountKey, prefix+"azure.account-key", "", "Azure storage account key")
	f.StringVar(&cfg.ContainerName, prefix+"azure.container-name", "", "Azure storage container name")
	f.StringVar(&cfg.Endpoint, prefix+"azure.endpoint-suffix", "", "Azure storage endpoint suffix without schema. The account name will be prefixed to this value to create the FQDN")
	f.Int32Var(&cfg.MaxRetries, prefix+"azure.max-retries", 20, "Number of retries for recoverable errors")
	f.StringVar(&cfg.MsiResource, prefix+"azure.msi-resource", "", "Azure storage MSI resource. Either this or account key must be set.")
	f.StringVar(&cfg.UserAssignedID, prefix+"azure.user-assigned-id", "", "Azure storage MSI resource managed identity client Id. If not supplied system assigned identity is used")
	cfg.Http.RegisterFlagsWithPrefix(prefix+"azure.", f)
}

func (cfg *SwiftStorageSpec) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.Int32Var(&cfg.AuthVersion, prefix+"swift.auth-version", 0, "OpenStack Swift authentication API version. 0 to autodetect.")
	f.StringVar(&cfg.AuthURL, prefix+"swift.auth-url", "", "OpenStack Swift authentication URL")
	f.StringVar(&cfg.Username, prefix+"swift.username", "", "OpenStack Swift username.")
	f.StringVar(&cfg.UserDomainName, prefix+"swift.user-domain-name", "", "OpenStack Swift user's domain name.")
	f.StringVar(&cfg.UserDomainID, prefix+"swift.user-domain-id", "", "OpenStack Swift user's domain ID.")
	f.StringVar(&cfg.UserID, prefix+"swift.user-id", "", "OpenStack Swift user ID.")
	f.StringVar(&cfg.Password, prefix+"swift.password", "", "OpenStack Swift API key.")
	f.StringVar(&cfg.DomainID, prefix+"swift.domain-id", "", "OpenStack Swift user's domain ID.")
	f.StringVar(&cfg.DomainName, prefix+"swift.domain-name", "", "OpenStack Swift user's domain name.")
	f.StringVar(&cfg.ProjectID, prefix+"swift.project-id", "", "OpenStack Swift project ID (v2,v3 auth only).")
	f.StringVar(&cfg.ProjectName, prefix+"swift.project-name", "", "OpenStack Swift project name (v2,v3 auth only).")
	f.StringVar(&cfg.ProjectDomainID, prefix+"swift.project-domain-id", "", "ID of the OpenStack Swift project's domain (v3 auth only), only needed if it differs the from user domain.")
	f.StringVar(&cfg.ProjectDomainName, prefix+"swift.project-domain-name", "", "Name of the OpenStack Swift project's domain (v3 auth only), only needed if it differs from the user domain.")
	f.StringVar(&cfg.RegionName, prefix+"swift.region-name", "", "OpenStack Swift Region to use (v2,v3 auth only).")
	f.StringVar(&cfg.ContainerName, prefix+"swift.container-name", "", "Name of the OpenStack Swift container to put chunks in.")
	f.Int32Var(&cfg.MaxRetries, prefix+"swift.max-retries", 3, "Max retries on requests error.")
	f.Var(dpbValue(10*time.Second, &cfg.ConnectTimeout), prefix+"swift.connect-timeout", "Time after which a connection attempt is aborted.")
	f.Var(dpbValue(5*time.Second, &cfg.RequestTimeout), prefix+"swift.request-timeout", "Time after which an idle request is aborted. The timeout watchdog is reset each time some data is received, so the timeout triggers after X time no data is received on a request.")
}

func (cfg *FilesystemStorageSpec) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.StringVar(&cfg.Directory, prefix+"filesystem.dir", "/data", "Local filesystem storage directory.")
}

func (cfg *HTTPConfig) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.Var(dpbValue(90*time.Second, &cfg.IdleConnTimeout), prefix+"http.idle-conn-timeout", "The time an idle connection will remain idle before closing.")
	f.Var(dpbValue(2*time.Minute, &cfg.ResponseHeaderTimeout), prefix+"http.response-header-timeout", "The amount of time the client will wait for a servers response headers.")
	f.BoolVar(&cfg.InsecureSkipVerify, prefix+"http.insecure-skip-verify", false, "If the client connects via HTTPS and this option is enabled, the client will accept any certificate and hostname.")
	f.Var(dpbValue(10*time.Second, &cfg.TlsHandshakeTimeout), prefix+"tls-handshake-timeout", "Maximum time to wait for a TLS handshake. 0 means no limit.")
	f.Var(dpbValue(1*time.Second, &cfg.ExpectContinueTimeout), prefix+"expect-continue-timeout", "The time to wait for a server's first response headers after fully writing the request headers if the request has an Expect header. 0 to send the request body immediately.")
	f.Int32Var(&cfg.MaxIdleConns, prefix+"max-idle-connections", 100, "Maximum number of idle (keep-alive) connections across all hosts. 0 means no limit.")
	f.Int32Var(&cfg.MaxIdleConnsPerHost, prefix+"max-idle-connections-per-host", 100, "Maximum number of idle (keep-alive) connections to keep per-host. If 0, a built-in default value is used.")
	f.Int32Var(&cfg.MaxConnsPerHost, prefix+"max-connections-per-host", 0, "Maximum number of connections per host. 0 means no limit.")
}

func (cfg *SSEConfig) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.StringVar(&cfg.Type, prefix+"type", "", fmt.Sprintf("Enable AWS Server Side Encryption. Supported values: %s.", strings.Join(supportedSSETypes, ", ")))
	f.StringVar(&cfg.KmsKeyID, prefix+"kms-key-id", "", "KMS Key ID used to encrypt objects in S3")
	f.StringVar(&cfg.KmsEncryptionContext, prefix+"kms-encryption-context", "", "KMS Encryption Context used for object encryption. It expects JSON formatted string.")
}

func (cfg *StorageSpec) InitEmptyFields() {
	if cfg.S3 == nil {
		cfg.S3 = &S3StorageSpec{}
	}
	cfg.S3.InitEmptyFields()
	if cfg.Gcs == nil {
		cfg.Gcs = &GCSStorageSpec{}
	}
	cfg.Gcs.InitEmptyFields()
	if cfg.Azure == nil {
		cfg.Azure = &AzureStorageSpec{}
	}
	cfg.Azure.InitEmptyFields()
	if cfg.Swift == nil {
		cfg.Swift = &SwiftStorageSpec{}
	}
	cfg.Swift.InitEmptyFields()
	if cfg.Filesystem == nil {
		cfg.Filesystem = &FilesystemStorageSpec{}
	}
	cfg.Filesystem.InitEmptyFields()
}

func (cfg *S3StorageSpec) InitEmptyFields() {
	if cfg.Sse == nil {
		cfg.Sse = &SSEConfig{}
	}
	if cfg.Http == nil {
		cfg.Http = &HTTPConfig{}
	}
}

func (cfg *GCSStorageSpec) InitEmptyFields() {

}

func (cfg *AzureStorageSpec) InitEmptyFields() {
	if cfg.Http == nil {
		cfg.Http = &HTTPConfig{}
	}
}

func (cfg *SwiftStorageSpec) InitEmptyFields() {

}

func (cfg *FilesystemStorageSpec) InitEmptyFields() {

}
