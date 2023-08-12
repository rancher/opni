package configutil

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"

	gokitlog "github.com/go-kit/kit/log"
	"github.com/minio/minio-go/v7/pkg/s3utils"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/reflect/protopath"

	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
)

type errLogger struct {
	mu   sync.Mutex
	errs []string
}

func (l *errLogger) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errs = append(l.errs, string(b))
	return len(b), nil
}

func mkwarningf(msg string, args ...any) *cortexops.ValidationError {
	return &cortexops.ValidationError{
		Severity: cortexops.ValidationError_Warning,
		Message:  fmt.Sprintf(msg, args...),
	}
}

func mkerrorf(msg string, args ...any) *cortexops.ValidationError {
	return &cortexops.ValidationError{
		Severity: cortexops.ValidationError_Error,
		Message:  fmt.Sprintf(msg, args...),
	}
}

func requiredFieldError(field protopath.Path) *cortexops.ValidationError {
	return mkerrorf("missing required field: %s", field)
}

func ValidateConfiguration(cfg *cortexops.CapabilityBackendConfigSpec, overriders ...CortexConfigOverrider) []*cortexops.ValidationError {
	errs := []*cortexops.ValidationError{}
	errs = append(errs, CollectValidationErrorLogs(cfg.GetCortexConfig(), overriders...)...)
	errs = append(errs, RunCustomValidationRules(cfg)...)
	return errs
}

func CollectValidationErrorLogs(cfg *cortexops.CortexApplicationConfig, overriders ...CortexConfigOverrider) []*cortexops.ValidationError {
	errs := []*cortexops.ValidationError{}
	conf, _, err := CortexAPISpecToCortexConfig(cfg, overriders...)
	if err != nil {
		errs = append(errs, mkerrorf(err.Error()))
		for _, err := range errs {
			err.Source = "opni"
		}
		return errs
	}
	lg := errLogger{}
	errLogger := gokitlog.NewJSONLogger(&lg)
	if err := conf.Validate(errLogger); err != nil {
		errs = append(errs, mkerrorf(err.Error()))
	}
	for _, err := range lg.errs {
		errs = append(errs, mkerrorf(err))
	}
	for _, err := range errs {
		err.Source = "cortex"
	}
	return errs
}

func RunCustomValidationRules(cfg *cortexops.CapabilityBackendConfigSpec) []*cortexops.ValidationError {
	rules := []func(*cortexops.CapabilityBackendConfigSpec) []*cortexops.ValidationError{
		validateFilesystemStorageModeUsage,
		validateRequiredStorageCredentials,
		validateTargets,
	}
	errs := []*cortexops.ValidationError{}
	for _, rule := range rules {
		errs = append(errs, rule(cfg)...)
	}
	for _, err := range errs {
		err.Source = "opni"
	}
	return errs
}

// ensures that if the filesystem backend is used, all-in-one mode is also used (only one "all" target present)
func validateFilesystemStorageModeUsage(cfg *cortexops.CapabilityBackendConfigSpec) []*cortexops.ValidationError {
	if cfg.GetCortexConfig().GetStorage().GetBackend() == storagev1.Filesystem {
		targets := cfg.GetCortexWorkloads().GetTargets()
		if len(targets) == 0 {
			return nil
		}
		if len(targets) != 1 || targets["all"] == nil {
			return []*cortexops.ValidationError{
				mkerrorf(`filesystem storage backend can only be used with a single "all" target`),
			}
		}
	}
	return nil
}

// ensures that the required credentials are present for the selected storage backend
func validateRequiredStorageCredentials(cfg *cortexops.CapabilityBackendConfigSpec) []*cortexops.ValidationError {
	backend := cfg.GetCortexConfig().GetStorage().GetBackend()
	var errs []*cortexops.ValidationError
	switch backend {
	case storagev1.S3:
		conf := cfg.GetCortexConfig().GetStorage().GetS3()
		if err := validateEndpointUrl(conf.GetEndpoint(), conf.GetInsecure()); err != nil {
			errs = append(errs, err)
		}
		if conf.GetAccessKeyId() == "" {
			errs = append(errs, mkerrorf("s3: access_key_id is required"))
		}
		if conf.GetSecretAccessKey() == "" {
			errs = append(errs, mkerrorf("s3: secret_access_key is required"))
		}
		if conf.GetBucketName() == "" {
			errs = append(errs, mkerrorf("s3: bucket_name is required"))
		}
	case storagev1.GCS:
		conf := cfg.GetCortexConfig().GetStorage().GetGcs()
		if conf.GetBucketName() == "" {
			errs = append(errs, mkerrorf("gcs: bucket_name is required"))
		}
		if conf.GetServiceAccount() == "" {
			errs = append(errs, mkerrorf("gcs: service_account is required"))
		}
	case storagev1.Azure:
		conf := cfg.GetCortexConfig().GetStorage().GetAzure()
		if conf.GetMsiResource() == "" && conf.GetAccountKey() == "" {
			errs = append(errs, mkerrorf("azure: msi_resource or account_key is required"))
		}
		if conf.GetMsiResource() != "" && conf.GetAccountKey() != "" {
			errs = append(errs, mkerrorf("azure: msi_resource and account_key are mutually exclusive"))
		}
		if conf.GetUserAssignedId() != "" && conf.GetAccountKey() != "" {
			errs = append(errs, mkerrorf("azure: user_assigned_id cannot be set when using account_key authentication"))
		}
		if conf.GetAccountName() == "" {
			errs = append(errs, mkerrorf("azure: account_name is required"))
		}
		if conf.GetContainerName() == "" {
			errs = append(errs, mkerrorf("azure: container_name is required"))
		}
		if conf.GetMaxRetries() < 0 {
			errs = append(errs, mkerrorf("azure: max_retries must be greater than or equal to 0"))
		}
	case storagev1.Swift:
		conf := cfg.GetCortexConfig().GetStorage().GetSwift()
		if conf.GetAuthUrl() == "" {
			errs = append(errs, mkerrorf("swift: auth_url is required"))
		}
		username := conf.GetUsername()
		userId := conf.GetUserId()
		appCredId := conf.GetApplicationCredentialId()
		appCredName := conf.GetApplicationCredentialName()
		hasUsernameAuth := (username != "" || userId != "")
		hasApplicationCredAuth := (appCredId != "" || appCredName != "")

		var whichUser []string
		if username != "" {
			whichUser = append(whichUser, "username")
		}
		if userId != "" {
			whichUser = append(whichUser, "user_id")
		}

		var whichAppCred []string
		if appCredId != "" {
			whichAppCred = append(whichAppCred, "application_credential_id")
		}
		if appCredName != "" {
			whichAppCred = append(whichAppCred, "application_credential_name")
		}

		switch {
		case hasUsernameAuth && hasApplicationCredAuth:
			errs = append(errs, mkerrorf("swift: %s %s mutually exclusive with %s",
				strings.Join(whichUser, " and "), lo.Ternary(len(whichUser) > 1, "are", "is"),
				strings.Join(whichAppCred, " and ")))
		case !hasUsernameAuth && !hasApplicationCredAuth:
			errs = append(errs, mkerrorf("swift: one of {username|user_id|application_credential_name|application_credential_id} is required"))
		case hasUsernameAuth:
			if conf.GetPassword() == "" {
				errs = append(errs, mkerrorf("swift: password is required when username or user_id is set"))
			}
		case hasApplicationCredAuth:
			if conf.GetApplicationCredentialSecret() == "" {
				errs = append(errs, mkerrorf("swift: application_credential_secret is required when application_credential_id or application_credential_name is set"))
			}
		}
	case storagev1.Filesystem:
		conf := cfg.GetCortexConfig().GetStorage().GetFilesystem()
		if conf.GetDir() == "" {
			errs = append(errs, mkerrorf("filesystem: dir is required"))
		}
	}
	return errs
}

// enforces mutual exclusivity of the "all" target with other targets, and validates target names
func validateTargets(cfg *cortexops.CapabilityBackendConfigSpec) []*cortexops.ValidationError {
	var errs []*cortexops.ValidationError
	targets := cfg.GetCortexWorkloads().GetTargets()
	if len(targets) == 0 {
		errs = append(errs, mkwarningf("no targets specified; cortex will not be deployed"))
	}
	if targets["all"] != nil && len(targets) > 1 {
		errs = append(errs, mkerrorf(`if the "all" target is present, it must be the only target`))
	}
	for name := range targets {
		if name == "all" {
			continue
		}
		if !slices.Contains(cortexTargets, name) {
			errs = append(errs, mkerrorf("invalid target name: %q", name))
		}
	}
	if len(targets) > 1 {
		for _, target := range cortexTargets {
			if t, ok := targets[target]; !ok {
				errs = append(errs, mkwarningf("target %q is not configured", target))
			} else {
				if t.GetReplicas() == 0 {
					errs = append(errs, mkwarningf("target %q has 0 replicas", target))
				}
				for _, extraArg := range t.GetExtraArgs() {
					if !strings.HasPrefix(extraArg, "-") {
						errs = append(errs, mkerrorf("malformed extra argument for target %q: %q (expecting '-' or '--' prefix)", target, extraArg))
					} else {
						trimmedArg := strings.TrimLeft(extraArg, "-")
						if strings.HasPrefix(trimmedArg, "target") ||
							strings.HasPrefix(trimmedArg, "config") ||
							strings.HasPrefix(trimmedArg, "modules") ||
							strings.HasPrefix(trimmedArg, "help") {
							errs = append(errs, mkerrorf("invalid extra argument for target %q: %q", target, extraArg))
						}
					}
				}
			}
		}
	}
	return errs
}

// adapted from github.com/minio/minio-go/blob/master/utils.go (used by thanos/cortex)
func validateEndpointUrl(endpoint string, insecure bool) *cortexops.ValidationError {
	if endpoint == "" {
		return mkwarningf("s3: endpoint url is required")
	}

	scheme := "https"
	if insecure {
		scheme = "http"
	}

	endpointURL, err := url.Parse(fmt.Sprintf("%s://%s", scheme, endpoint))
	if err != nil {
		return mkwarningf("s3: endpoint url is invalid: %v", err)
	}

	if *endpointURL == (url.URL{}) {
		return mkwarningf("s3: endpoint url cannot be empty")
	}
	if endpointURL.Path != "/" && endpointURL.Path != "" {
		return mkwarningf("s3: endpoint url cannot have fully qualified paths (expected '/' or empty path, got %q)", endpointURL.Path)
	}
	host := endpointURL.Hostname()
	validIP := s3utils.IsValidIP(host)
	validDomain := s3utils.IsValidDomain(host)
	if !validIP && !validDomain {
		if err := detailedInvalidHostError(host); err != nil {
			return err
		} else if err := net.ParseIP(host); err != nil {
			return mkwarningf("s3: malformed endpoint: %v", err)
		}
	}

	if strings.Contains(host, ".s3.amazonaws.com") {
		if !s3utils.IsAmazonEndpoint(*endpointURL) {
			return mkwarningf("s3: endpoint should be 's3.amazonaws.com' without the region prefix")
		}
	}
	if strings.Contains(host, ".googleapis.com") {
		if !s3utils.IsGoogleEndpoint(*endpointURL) {
			return mkwarningf("s3: endpoint should be 'storage.googleapis.com' without the region prefix")
		}
	}
	return nil
}
func detailedInvalidHostError(host string) *cortexops.ValidationError {
	// See RFC 1035, RFC 3696.
	host = strings.TrimSpace(host)
	if len(host) == 0 {
		return mkwarningf("host cannot be empty")
	}
	if len(host) > 255 {
		return mkwarningf("host cannot be longer than 255 characters")
	}

	if host[len(host)-1:] == "-" || host[:1] == "-" {
		return mkwarningf("host cannot start or end with '-'")
	}
	if host[len(host)-1:] == "_" || host[:1] == "_" {
		return mkwarningf("host cannot start or end with '_'")
	}
	if host[:1] == "." {
		return mkwarningf("host cannot start with '.'")
	}
	if i := strings.IndexAny(host, "`~!@#$%^&*()+={}[]|\\\"';:><?/"); i >= 0 {
		return mkwarningf("host contains invalid character '%c'", rune(host[i]))
	}
	return nil
}
