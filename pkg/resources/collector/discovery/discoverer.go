package discovery

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"log/slog"

	promoperatorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringv1beta1 "github.com/rancher/opni/apis/monitoring/v1beta1"

	promcommon "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/relabel"

	"github.com/samber/lo"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	TlsAssetMountPath = "/etc/otel/prometheus/certs"
	overrideJobName   = "__tmp_prometheus_job_name"
)

var (
	// somewhat counter-intuitively, decreasing this increases memory usage
	opniDefaultScrapeInterval = model.Duration(60 * time.Second)
	opniDefaultScrapeTimeout  = model.Duration(30 * time.Second)
)

type target struct {
	staticAddress string
	friendlyName  string
}

func (t target) Less(other target) bool {
	// handles the case where we have services manageing different ports on the same sets of pods for example
	if t.staticAddress == other.staticAddress {
		return t.friendlyName < other.friendlyName
	}
	return t.staticAddress < other.staticAddress
}

func (t target) Compare(other target) int {
	if t == other {
		return 0
	}
	if t.Less(other) {
		return -1
	}
	return 1
}

type PrometheusDiscovery struct {
	logger     *slog.Logger
	retrievers []ScrapeConfigRetriever
}

type jobs = map[string]yaml.MapSlice

type promCRDOperatorConfig struct {
	jobs map[string]yaml.MapSlice
	// we must combine these secrets into one secret "tls-assets"
	secrets []SecretResolutionConfig
}

// How to resolve a required secret or config map for scraping
// Secret and ConfigMap are mutually exclusive
type SecretResolutionConfig struct {
	namespace string
	TargetKey string
	*corev1.Secret
	*corev1.ConfigMap
}

func (s *SecretResolutionConfig) isEmpty() bool {
	return s.TargetKey == "" && s.Secret == nil && s.ConfigMap == nil
}

func (s *SecretResolutionConfig) Key() string {
	if s.isEmpty() {
		return ""
	}
	if s.Secret != nil {
		return fmt.Sprintf("secret_%s_%s_%s", s.namespace, s.Secret.Name, s.TargetKey)
	}
	if s.ConfigMap != nil {
		return fmt.Sprintf("config_%s_%s_%s", s.namespace, s.ConfigMap.Name, s.TargetKey)
	}
	return fmt.Sprintf("invalid_%s_%s", s.namespace, s.TargetKey)
}

func (s *SecretResolutionConfig) Path() string {
	if s.isEmpty() {
		return ""
	}
	return fmt.Sprintf("%s/%s", TlsAssetMountPath, s.Key())
}

func (s *SecretResolutionConfig) GetData() []byte {
	if s.Secret != nil {
		return s.Secret.Data[s.TargetKey]
	}
	if s.ConfigMap != nil {
		return []byte(s.ConfigMap.Data[s.TargetKey])
	}
	return []byte{}
}

type ScrapeConfigRetriever interface {
	Yield() (*promCRDOperatorConfig, error)
	Name() string
}

func NewPrometheusDiscovery(
	logger *slog.Logger,
	client client.Client,
	namespace string,
	discovery monitoringv1beta1.PrometheusDiscovery,
) PrometheusDiscovery {
	return PrometheusDiscovery{
		logger: logger,
		retrievers: []ScrapeConfigRetriever{
			NewServiceMonitorScrapeConfigRetriever(logger, client, namespace, discovery),
			NewPodMonitorScrapeConfigRetriever(logger, client, namespace, discovery),
		},
	}
}

func (p *PrometheusDiscovery) YieldScrapeConfigs() (
	scrapeCfgs []yaml.MapSlice,
	secrets []SecretResolutionConfig,
	retErr error,
) {
	cfgChan := make(chan *promCRDOperatorConfig)

	go func() {
		var wg sync.WaitGroup
		defer close(cfgChan)
		for _, retriever := range p.retrievers {
			retriever := retriever
			lg := p.logger.With("collector", retriever.Name())
			wg.Add(1)
			go func() {
				defer wg.Done()
				cfg, err := retriever.Yield()
				if err != nil {
					lg.Warn(fmt.Sprintf("failed to retrieve scrape configs : %s", err))
					return
				}
				lg.Info(fmt.Sprintf("found %d scrape configs", len(cfg.jobs)))
				lg.Info(fmt.Sprintf("found %d secrets to combine and mount", len(cfg.secrets)))
				cfgChan <- cfg
			}()
		}
		wg.Wait()

	}()
	jobMap := map[string]yaml.MapSlice{}
	secretMap := map[string]SecretResolutionConfig{}
	for {
		cfg, ok := <-cfgChan
		if cfg != nil && len(cfg.jobs) > 0 {
			jobMap = lo.Assign(jobMap, cfg.jobs)
		}
		if cfg != nil && len(cfg.secrets) > 0 {
			for _, secret := range cfg.secrets {
				secretMap[secret.Key()] = secret
			}
		}
		if !ok {
			break
		}
	}
	jobs := lo.Keys(jobMap)
	sort.Strings(jobs)
	for _, job := range jobs {
		scrapeCfgs = append(scrapeCfgs, jobMap[job])
	}
	secretKeys := lo.Keys(secretMap)
	sort.Strings(secretKeys)
	for _, secretKey := range secretKeys {
		secrets = append(secrets, secretMap[secretKey])
	}
	p.logger.Debug(fmt.Sprintf("reduced found scrape configs to %d", len(scrapeCfgs)))
	p.logger.Debug(fmt.Sprintf("reduced found tls configs to %d", len(secrets)))
	return scrapeCfgs, secrets, nil
}

func parseStrToDuration(s string) model.Duration {
	dur, err := time.ParseDuration(s)
	if err != nil {
		return model.Duration(time.Minute)
	}
	return model.Duration(dur)
}

func fromSafeAuthorization(
	client client.Client,
	safeCfg *promoperatorv1.SafeAuthorization,
	lg *slog.Logger,
	namespace string,
) (cfg promcommon.Authorization, secretRes []SecretResolutionConfig) {
	secretResolution := SecretResolutionConfig{}
	if safeCfg.Credentials != nil {
		credentialSecret := &corev1.Secret{}
		err := client.Get(context.TODO(), types.NamespacedName{
			Name:      safeCfg.Credentials.Name,
			Namespace: namespace,
		}, credentialSecret)
		if err == nil {
			secretResolution = SecretResolutionConfig{
				TargetKey: safeCfg.Credentials.Key,
				Secret:    credentialSecret,
				namespace: namespace,
			}
			secretRes = append(secretRes, secretResolution)
		} else {
			lg.Warn(fmt.Sprintf("failed to get secret %s/%s : %s", namespace, safeCfg.Credentials.Name, err))
		}
	}
	return promcommon.Authorization{
		Type:            safeCfg.Type,
		CredentialsFile: secretResolution.Path(),
	}, secretRes
}

func fromSafeTlsConfig(
	client client.Client,
	safeCfg promoperatorv1.SafeTLSConfig,
	lg *slog.Logger,
	namespace string,
) (cfg promcommon.TLSConfig, secretRes []SecretResolutionConfig) {
	caSecretResolution := SecretResolutionConfig{}
	if safeCfg.CA.Secret != nil {
		caSecret := &corev1.Secret{}
		err := client.Get(context.TODO(), types.NamespacedName{
			Name:      safeCfg.CA.Secret.Name,
			Namespace: namespace,
		}, caSecret)
		if err == nil {
			caSecretResolution = SecretResolutionConfig{
				TargetKey: safeCfg.CA.Secret.Key,
				Secret:    caSecret,
				namespace: namespace,
			}
		} else {
			lg.Warn(fmt.Sprintf("failed to find a specified ca secret '%v' : %s", safeCfg.CA.Secret, err))
		}
	}
	if safeCfg.CA.ConfigMap != nil {
		caConfig := &corev1.ConfigMap{}
		err := client.Get(context.TODO(), types.NamespacedName{
			Name:      safeCfg.CA.Secret.Name,
			Namespace: namespace,
		}, caConfig)
		if err == nil {
			caSecretResolution = SecretResolutionConfig{
				TargetKey: safeCfg.CA.ConfigMap.Key,
				ConfigMap: caConfig,
				namespace: namespace,
			}
		} else {
			lg.Warn(fmt.Sprintf("failed to find a specified ca configmap '%v' : %s", safeCfg.CA.ConfigMap, err))
		}
	}
	certSecretResolution := SecretResolutionConfig{}
	if safeCfg.Cert.Secret != nil {
		certSecret := &corev1.Secret{}
		err := client.Get(context.TODO(), types.NamespacedName{
			Name:      safeCfg.Cert.Secret.Name,
			Namespace: namespace,
		}, certSecret)
		if err == nil {
			certSecretResolution = SecretResolutionConfig{
				TargetKey: safeCfg.Cert.Secret.Key,
				Secret:    certSecret,
				namespace: namespace,
			}
		} else {
			lg.Warn(fmt.Sprintf("failed to find a specified cert secret '%v' : %s", safeCfg.Cert.Secret, err))
		}
	}
	if safeCfg.Cert.ConfigMap != nil {
		certConfig := &corev1.ConfigMap{}
		err := client.Get(context.TODO(), types.NamespacedName{
			Name:      safeCfg.Cert.Secret.Name,
			Namespace: namespace,
		}, certConfig)
		if err == nil {
			certSecretResolution = SecretResolutionConfig{
				TargetKey: safeCfg.Cert.ConfigMap.Key,
				ConfigMap: certConfig,
				namespace: namespace,
			}
		} else {
			lg.Warn(fmt.Sprintf("failed to find a specified cert configmap '%v' : %s", safeCfg.Cert.ConfigMap, err))
		}
	}
	keySecretResolution := SecretResolutionConfig{}
	if safeCfg.KeySecret != nil {
		keySecret := &corev1.Secret{}
		err := client.Get(context.TODO(), types.NamespacedName{
			Namespace: namespace,
			Name:      safeCfg.KeySecret.Name,
		}, keySecret)
		if err == nil {
			keySecretResolution = SecretResolutionConfig{
				TargetKey: safeCfg.KeySecret.Key,
				Secret:    keySecret,
				namespace: namespace,
			}
		} else {
			lg.Warn(fmt.Sprintf("failed to find a specified Key secret : %v", safeCfg.KeySecret))
		}
	}

	cfg = promcommon.TLSConfig{
		CAFile:             caSecretResolution.Path(),
		CertFile:           certSecretResolution.Path(),
		KeyFile:            keySecretResolution.Path(),
		ServerName:         safeCfg.ServerName,
		InsecureSkipVerify: safeCfg.InsecureSkipVerify,
	}
	if !caSecretResolution.isEmpty() {
		secretRes = append(secretRes, caSecretResolution)
	}
	if !certSecretResolution.isEmpty() {
		secretRes = append(secretRes, certSecretResolution)
	}
	if !keySecretResolution.isEmpty() {
		secretRes = append(secretRes, keySecretResolution)
	}
	return
}

func isEmpty(sf promoperatorv1.SafeTLSConfig) bool {
	return sf.KeySecret == nil &&
		(sf.CA.ConfigMap == nil && sf.CA.Secret == nil) &&
		(sf.Cert.ConfigMap == nil && sf.Cert.Secret == nil)
}

func jobRelabelling(newJobName string) []*relabel.Config {
	return []*relabel.Config{
		{
			TargetLabel: "job",
			Replacement: newJobName,
		},
	}
}

func generateRelabelConfig(rc []*promoperatorv1.RelabelConfig) []*relabel.Config {
	cfg := []*relabel.Config{}
	for _, c := range rc {
		relabeling := &relabel.Config{}
		if len(c.SourceLabels) > 0 {
			relabeling.SourceLabels = lo.Map(c.SourceLabels, func(r promoperatorv1.LabelName, _ int) model.LabelName {
				return model.LabelName(r)
			})
		}
		if c.Separator != "" {
			relabeling.Separator = c.Separator
		}
		if c.TargetLabel != "" {
			relabeling.TargetLabel = c.TargetLabel
		}
		if c.Regex != "" {
			rRe, err := relabel.NewRegexp(c.Regex)
			if err != nil {
				panic(err) // panic for now
			}
			relabeling.Regex = rRe
		}
		if c.Modulus != uint64(0) {
			relabeling.Modulus = c.Modulus
		}
		if c.Replacement != "" {
			relabeling.Replacement = c.Replacement
		}
		if c.Action != "" {
			relabeling.Action = relabel.Action(strings.ToLower(c.Action))
		}
		cfg = append(cfg, relabeling)
	}
	return cfg
}
