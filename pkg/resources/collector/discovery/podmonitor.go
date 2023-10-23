package discovery

import (
	"context"
	"fmt"

	"slices"

	"log/slog"

	promcommon "github.com/prometheus/common/config"
	promcfg "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/samber/lo"
	"gopkg.in/yaml.v2"

	promoperatorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringv1beta1 "github.com/rancher/opni/apis/monitoring/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type podMonitorScrapeConfigRetriever struct {
	client client.Client
	logger *slog.Logger
	// current namespace the collector is defined in.
	// it should only discoverer secrets for TLS/auth in this namespace.
	namespace string
	discovery monitoringv1beta1.PrometheusDiscovery
}

func NewPodMonitorScrapeConfigRetriever(
	logger *slog.Logger,
	client client.Client,
	namespace string,
	discovery monitoringv1beta1.PrometheusDiscovery,
) ScrapeConfigRetriever {
	return &podMonitorScrapeConfigRetriever{
		client:    client,
		namespace: namespace,
		logger:    logger,
		discovery: discovery,
	}
}

func (p *podMonitorScrapeConfigRetriever) Name() string {
	return "podMonitor"
}

func (p *podMonitorScrapeConfigRetriever) resolvePodMonitor(ns []string) (*promoperatorv1.PodMonitorList, error) {
	podMonitorList := &promoperatorv1.PodMonitorList{}
	if len(ns) == 0 {
		listOptions := &client.ListOptions{
			Namespace: metav1.NamespaceAll,
		}
		err := p.client.List(
			context.TODO(),
			podMonitorList,
			listOptions,
		)
		if err != nil {
			return nil, err
		}
		return podMonitorList, nil
	} else {
		for _, n := range ns {
			listOptions := &client.ListOptions{
				Namespace: n,
			}
			err := p.client.List(
				context.TODO(),
				podMonitorList,
				listOptions,
			)
			if err != nil {
				p.logger.Warn(fmt.Sprintf("failed to list pod monitors %s", err))
				continue
			}
			podMonitorList.Items = append(podMonitorList.Items, podMonitorList.Items...)
		}
	}
	return &promoperatorv1.PodMonitorList{}, nil
}

func (p *podMonitorScrapeConfigRetriever) Yield() (cfg *promCRDOperatorConfig, retErr error) {
	podMonitorList, err := p.resolvePodMonitor(p.discovery.NamespaceSelector)
	if err != nil {
		return nil, err
	}
	pMons := lo.Associate(podMonitorList.Items, func(item *promoperatorv1.PodMonitor) (string, *promoperatorv1.PodMonitor) {
		return item.ObjectMeta.Name + "-" + item.ObjectMeta.Namespace, item
	})

	// jobName -> scrapeConfigs
	cfgMap := jobs{}
	secretRes := []SecretResolutionConfig{}
	p.logger.Info(fmt.Sprintf("found %d pod monitors", len(pMons)))
	for _, pMon := range pMons {
		lg := p.logger.With("podMonitor", pMon.Namespace+"-"+pMon.Name)
		selectorMap, err := metav1.LabelSelectorAsMap(&pMon.Spec.Selector)
		if err != nil {
			continue
		}
		nSel := pMon.Spec.NamespaceSelector
		podList := &corev1.PodList{}
		if nSel.Any || len(nSel.MatchNames) == 0 {
			pList := &corev1.PodList{}
			listOptions := &client.ListOptions{
				Namespace:     metav1.NamespaceAll,
				LabelSelector: labels.SelectorFromSet(selectorMap),
			}
			err = p.client.List(context.TODO(), pList, listOptions)
			if err != nil {
				lg.Warn(fmt.Sprintf("failed to select pods for pod monitor %s", selectorMap))
				continue
			}
			podList.Items = append(podList.Items, pList.Items...)
		} else {
			ns := &corev1.NamespaceList{}
			err := p.client.List(context.TODO(), ns)
			if err != nil {
				continue
			}
			toMatch := []string{}
			for _, n := range ns.Items {
				if slices.Contains(nSel.MatchNames, n.Name) {
					toMatch = append(toMatch, n.Name)
				}
			}
			for _, ns := range toMatch {
				listOptions := &client.ListOptions{
					Namespace:     ns,
					LabelSelector: labels.SelectorFromSet(selectorMap),
				}
				pList := &corev1.PodList{}
				err = p.client.List(context.TODO(), pList, listOptions)
				if err != nil {
					continue
				}
				podList.Items = append(podList.Items, pList.Items...)
			}
		}
		numTargets := 0
		for i, ep := range pMon.Spec.PodMetricsEndpoints {
			targets := []target{}
			for _, pod := range podList.Items {
				podIP := pod.Status.PodIP
				for _, container := range pod.Spec.Containers {
					for _, port := range container.Ports {
						if port.Name != "" && ep.Port != "" && port.Name == ep.Port {
							targets = append(targets,
								target{
									staticAddress: fmt.Sprintf("%s:%d", podIP, port.ContainerPort),
									friendlyName:  p.generateFriendlyJobName(pMon, pod),
								})
						}
						if ep.TargetPort != nil {
							switch ep.TargetPort.Type {
							case intstr.Int:
								if port.ContainerPort == ep.TargetPort.IntVal {
									targets = append(targets, target{
										staticAddress: fmt.Sprintf("%s:%d", podIP, port.ContainerPort),
										friendlyName:  p.generateFriendlyJobName(pMon, pod),
									})
								}
							case intstr.String:
								if port.Name == ep.TargetPort.StrVal {
									targets = append(targets, target{
										staticAddress: fmt.Sprintf("%s:%d", podIP, port.ContainerPort),
										friendlyName:  p.generateFriendlyJobName(pMon, pod),
									})
								}
							}
						}
					}

				}
			}
			numTargets += len(targets)
			slices.SortFunc(targets, func(i, j target) int {
				return i.Compare(j)
			})
			if len(targets) > 0 {
				//de-dupe discovered targets
				job, sCfg, secrets := p.generateStaticPodConfig(pMon, ep, i, targets)
				if _, ok := cfgMap[job]; !ok {
					cfgMap[job] = sCfg
				}
				secretRes = append(secretRes, secrets...)
			}

		}
		if numTargets == 0 {
			lg.Warn("no targets found for pod monitor")
		}
	}

	return &promCRDOperatorConfig{
		jobs:    cfgMap,
		secrets: secretRes,
	}, nil
}

func (p *podMonitorScrapeConfigRetriever) generateStaticPodConfig(
	m *promoperatorv1.PodMonitor,
	ep promoperatorv1.PodMetricsEndpoint,
	i int,
	targets []target,
) (key string, out yaml.MapSlice, secretRes []SecretResolutionConfig) {
	uid := fmt.Sprintf("podMonitor/%s/%s/%d", m.Namespace, m.Name, i)
	cfg := promcfg.ScrapeConfig{
		// note : we need to treat this as uuid, the job name will get relabelled to its target job name
		// in the metric lifetime
		JobName:                 uid,
		ServiceDiscoveryConfigs: discovery.Configs{},
		MetricsPath:             ep.Path,
	}
	cfg.HonorLabels = ep.HonorLabels
	if ep.HonorTimestamps != nil {
		cfg.HonorTimestamps = *ep.HonorTimestamps
	}
	if ep.Interval != "" {
		dur := parseStrToDuration(string(ep.Interval))

		cfg.ScrapeInterval = dur
	}
	if ep.ScrapeTimeout != "" {
		dur := parseStrToDuration(string(ep.ScrapeTimeout))
		cfg.ScrapeTimeout = dur
	}
	if ep.Path != "" {
		cfg.MetricsPath = ep.Path
	}
	if ep.Scheme != "" {
		cfg.Scheme = ep.Scheme
	} else {
		cfg.Scheme = "http"
	}

	cfg.RelabelConfigs = append(cfg.RelabelConfigs, append(jobRelabelling(targets[0].friendlyName), generateRelabelConfig(ep.RelabelConfigs)...)...)
	cfg.MetricRelabelConfigs = append(cfg.MetricRelabelConfigs, generateRelabelConfig(ep.MetricRelabelConfigs)...)

	if ep.BearerTokenSecret.Name != "" {
		bearerSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ep.BearerTokenSecret.Name,
				Namespace: m.Namespace,
			},
		}
		err := p.client.Get(context.TODO(), types.NamespacedName{
			Name:      ep.BearerTokenSecret.Name,
			Namespace: m.Namespace,
		}, bearerSecret)
		if err == nil {
			bearerSecretRes := SecretResolutionConfig{
				TargetKey: ep.BearerTokenSecret.Key,
				namespace: p.namespace,
				Secret:    bearerSecret,
			}
			secretRes = append(secretRes, bearerSecretRes)
			cfg.HTTPClientConfig.Authorization = &promcommon.Authorization{
				Type:            "Bearer",
				CredentialsFile: bearerSecretRes.Path(),
			}
		} else {
			p.logger.Warn(fmt.Sprintf("failed to get expected bearer token secret %s for pod monitor %s", ep.BearerTokenSecret.Name, m.Name))
		}
	}

	if cfg.HTTPClientConfig.Authorization == nil {
		if ep.Authorization != nil {
			authorizationCfg, secrets := fromSafeAuthorization(
				p.client,
				ep.Authorization,
				p.logger,
				p.namespace,
			)
			secretRes = append(secretRes, secrets...)
			cfg.HTTPClientConfig.Authorization = &authorizationCfg
		}
	}

	if ep.TLSConfig != nil {
		if !isEmpty(ep.TLSConfig.SafeTLSConfig) {
			tlsCfg, secrets := fromSafeTlsConfig(
				p.client,
				ep.TLSConfig.SafeTLSConfig,
				p.logger,
				p.namespace,
			)
			secretRes = append(secretRes, secrets...)
			cfg.HTTPClientConfig.TLSConfig = tlsCfg
		}
	}

	bytes, err := yaml.Marshal(cfg)
	if err != nil {
		panic(err)
	}
	var ret yaml.MapSlice
	err = yaml.Unmarshal(bytes, &ret)
	if err != nil {
		panic(err)
	}

	sd := yaml.MapSlice{
		{
			Key:   "targets",
			Value: targets,
		},
	}

	staticConfig := yaml.MapItem{
		Key: "static_configs",
		Value: []yaml.MapSlice{
			sd,
		},
	}
	ret = append(ret, staticConfig)
	return uid, ret, secretRes
}

func (p *podMonitorScrapeConfigRetriever) generateFriendlyJobName(
	pm *promoperatorv1.PodMonitor,
	pod corev1.Pod,
) string {
	if pm.Spec.JobLabel != "" {
		if val, ok := pod.Labels[pm.Spec.JobLabel]; ok {
			return val
		}
	}
	return pod.Name
}
