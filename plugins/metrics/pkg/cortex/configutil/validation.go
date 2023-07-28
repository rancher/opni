package configutil

import (
	"fmt"
	"strings"
	"sync"

	gokitlog "github.com/go-kit/kit/log"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"

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

func CollectValidationErrorLogs(cfg *cortexops.CortexApplicationConfig) []*cortexops.ValidationError {
	errs := []*cortexops.ValidationError{}
	conf, _, err := CortexAPISpecToCortexConfig(cfg)
	if err != nil {
		errs = append(errs, mkerrorf(err.Error()))
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
		if conf.GetEndpoint() == "" {
			errs = append(errs, mkerrorf("endpoint is required"))
		}
		if conf.GetAccessKeyId() != "" {
			errs = append(errs, mkerrorf("access_key_id is required"))
		}
		if conf.GetSecretAccessKey() != "" {
			errs = append(errs, mkerrorf("secret_access_key is required"))
		}
		if conf.GetBucketName() == "" {
			errs = append(errs, mkerrorf("bucket_name is required"))
		}
	case storagev1.GCS:
		conf := cfg.GetCortexConfig().GetStorage().GetGcs()
		if conf.GetBucketName() == "" {
			errs = append(errs, mkerrorf("bucket_name is required"))
		}
		if conf.GetServiceAccount() == "" {
			errs = append(errs, mkerrorf("service_account is required"))
		}
	case storagev1.Azure:
		conf := cfg.GetCortexConfig().GetStorage().GetAzure()
		if conf.GetMsiResource() == "" && conf.GetAccountKey() == "" {
			errs = append(errs, mkerrorf("msi_resource or account_key is required"))
		}
		if conf.GetMsiResource() != "" && conf.GetAccountKey() != "" {
			errs = append(errs, mkerrorf("msi_resource and account_key are mutually exclusive"))
		}
		if conf.GetUserAssignedId() != "" && conf.GetAccountKey() != "" {
			errs = append(errs, mkerrorf("user_assigned_id cannot be set when using account_key authentication"))
		}
		if conf.GetAccountName() == "" {
			errs = append(errs, mkerrorf("account_name is required"))
		}
		if conf.GetContainerName() == "" {
			errs = append(errs, mkerrorf("container_name is required"))
		}
		if conf.GetMaxRetries() < 0 {
			errs = append(errs, mkerrorf("max_retries must be greater than or equal to 0"))
		}
	case storagev1.Swift:
		conf := cfg.GetCortexConfig().GetStorage().GetSwift()
		if conf.GetAuthUrl() == "" {
			errs = append(errs, mkerrorf("auth_url is required"))
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
			errs = append(errs, mkerrorf("%s %s mutually exclusive with %s",
				strings.Join(whichUser, " and "), lo.Ternary(len(whichUser) > 1, "are", "is"),
				strings.Join(whichAppCred, " and ")))
		case !hasUsernameAuth && !hasApplicationCredAuth:
			errs = append(errs, mkerrorf("one of {username|user_id|application_credential_name|application_credential_id} is required"))
		case hasUsernameAuth:
			if conf.GetPassword() == "" {
				errs = append(errs, mkerrorf("password is required when username or user_id is set"))
			}
		case hasApplicationCredAuth:
			if conf.GetApplicationCredentialSecret() == "" {
				errs = append(errs, mkerrorf("application_credential_secret is required when application_credential_id or application_credential_name is set"))
			}
		}
	case storagev1.Filesystem:
		conf := cfg.GetCortexConfig().GetStorage().GetFilesystem()
		if conf.GetDir() == "" {
			errs = append(errs, mkerrorf("dir is required"))
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
