package configutil

import (
	"fmt"
	"sync"

	gokitlog "github.com/go-kit/log"

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

func CollectValidationErrorLogs(cfg *cortexops.CortexApplicationConfig, overriders ...CortexConfigOverrider) []error {
	errs := []error{}
	conf, _, err := CortexAPISpecToCortexConfig(cfg, overriders...)
	if err != nil {
		errs = append(errs, fmt.Errorf("internal: %w", err))
		return errs
	}
	lg := errLogger{}
	errLogger := gokitlog.NewJSONLogger(&lg)
	if err := conf.Validate(errLogger); err != nil {
		errs = append(errs, fmt.Errorf("internal: %w", err))
	}
	for _, err := range lg.errs {
		errs = append(errs, fmt.Errorf("cortex: %s", err))
	}
	return errs
}

// func RunCustomValidationRules(cfg *cortexops.CapabilityBackendConfigSpec) []*driverutil.ValidationError {
// 	rules := []func(*cortexops.CapabilityBackendConfigSpec) []*driverutil.ValidationError{
// 		validateFilesystemStorageModeUsage,
// 		validateRequiredStorageCredentials,
// 		validateTargets,
// 	}
// 	errs := []*driverutil.ValidationError{}
// 	for _, rule := range rules {
// 		errs = append(errs, rule(cfg)...)
// 	}
// 	for _, err := range errs {
// 		err.Source = "opni"
// 	}
// 	return errs
// }

// // adapted from github.com/minio/minio-go/blob/master/utils.go (used by thanos/cortex)
// func validateEndpointUrl(endpoint string, insecure bool) *driverutil.ValidationError {
// 	if endpoint == "" {
// 		return mkwarningf("s3: endpoint url is required")
// 	}

// 	scheme := "https"
// 	if insecure {
// 		scheme = "http"
// 	}

// 	endpointURL, err := url.Parse(fmt.Sprintf("%s://%s", scheme, endpoint))
// 	if err != nil {
// 		return mkwarningf("s3: endpoint url is invalid: %v", err)
// 	}

// 	if *endpointURL == (url.URL{}) {
// 		return mkwarningf("s3: endpoint url cannot be empty")
// 	}
// 	if endpointURL.Path != "/" && endpointURL.Path != "" {
// 		return mkwarningf("s3: endpoint url cannot have fully qualified paths (expected '/' or empty path, got %q)", endpointURL.Path)
// 	}
// 	host := endpointURL.Hostname()
// 	validIP := s3utils.IsValidIP(host)
// 	validDomain := s3utils.IsValidDomain(host)
// 	if !validIP && !validDomain {
// 		if err := detailedInvalidHostError(host); err != nil {
// 			return err
// 		} else if err := net.ParseIP(host); err != nil {
// 			return mkwarningf("s3: malformed endpoint: %v", err)
// 		}
// 	}

// 	if strings.Contains(host, ".s3.amazonaws.com") {
// 		if !s3utils.IsAmazonEndpoint(*endpointURL) {
// 			return mkwarningf("s3: endpoint should be 's3.amazonaws.com' without the region prefix")
// 		}
// 	}
// 	if strings.Contains(host, ".googleapis.com") {
// 		if !s3utils.IsGoogleEndpoint(*endpointURL) {
// 			return mkwarningf("s3: endpoint should be 'storage.googleapis.com' without the region prefix")
// 		}
// 	}
// 	return nil
// }
// func detailedInvalidHostError(host string) *driverutil.ValidationError {
// 	// See RFC 1035, RFC 3696.
// 	host = strings.TrimSpace(host)
// 	if len(host) == 0 {
// 		return mkwarningf("host cannot be empty")
// 	}
// 	if len(host) > 255 {
// 		return mkwarningf("host cannot be longer than 255 characters")
// 	}

// 	if host[len(host)-1:] == "-" || host[:1] == "-" {
// 		return mkwarningf("host cannot start or end with '-'")
// 	}
// 	if host[len(host)-1:] == "_" || host[:1] == "_" {
// 		return mkwarningf("host cannot start or end with '_'")
// 	}
// 	if host[:1] == "." {
// 		return mkwarningf("host cannot start with '.'")
// 	}
// 	if i := strings.IndexAny(host, "`~!@#$%^&*()+={}[]|\\\"';:><?/"); i >= 0 {
// 		return mkwarningf("host contains invalid character '%c'", rune(host[i]))
// 	}
// 	return nil
// }
