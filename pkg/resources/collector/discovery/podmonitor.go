package discovery

import (
	"context"
	"fmt"

	promcfg "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v2"

	promoperatorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	// "k8s.io/apimachinery/pkg/runtime"
)

type podMonitorScrapeConfigRetriever struct {
	client client.Client
	logger *zap.SugaredLogger
}

func NewPodMonitorScrapeConfigRetriever(
	logger *zap.SugaredLogger,
	client client.Client,
) ScrapeConfigRetriever {
	return &podMonitorScrapeConfigRetriever{
		client: client,

		logger: logger,
	}
}

func (p *podMonitorScrapeConfigRetriever) Name() string {
	return "podMonitor"
}

func (p podMonitorScrapeConfigRetriever) Yield() (cfg jobs, retErr error) {
	listOptions := &client.ListOptions{
		Namespace: metav1.NamespaceAll,
	}
	podMonitorList := &promoperatorv1.PodMonitorList{}
	err := p.client.List(
		context.TODO(),
		podMonitorList,
		listOptions,
	)
	if err != nil {
		p.logger.Warnf("failed to list pod monitors %s", err)
		return nil, err
	}
	pMons := lo.Associate(podMonitorList.Items, func(item *promoperatorv1.PodMonitor) (string, *promoperatorv1.PodMonitor) {
		return item.ObjectMeta.Name + "-" + item.ObjectMeta.Namespace, item
	})

	// jobName -> scrapeConfig
	cfgMap := jobs{}
	p.logger.Debugf("found %d pod monitors", len(pMons))
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
				p.logger.Warnf("failed to select pods for pod monitor %s", selectorMap)
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

		for i, ep := range pMon.Spec.PodMetricsEndpoints {
			lg := lg.With("podEndpoint", fmt.Sprintf("%s/%s", ep.Port, ep.Path))
			targets := []string{}
			for _, pod := range podList.Items {
				podIP := pod.Status.PodIP
				for _, container := range pod.Spec.Containers {
					for _, port := range container.Ports {
						if port.Name != "" && ep.Port != "" && port.Name == ep.Port {
							targets = append(targets, fmt.Sprintf("%s:%d", podIP, port.ContainerPort))
						}
						if ep.TargetPort != nil {
							switch ep.TargetPort.Type {
							case intstr.Int:
								if port.ContainerPort == ep.TargetPort.IntVal {
									targets = append(targets, fmt.Sprintf("%s:%d", podIP, port.ContainerPort))
								}
							case intstr.String:
								if port.Name == ep.TargetPort.StrVal {
									targets = append(targets, fmt.Sprintf("%s:%d", podIP, port.ContainerPort))
								}
							}
						}
					}

				}
			}
			lg.Debugf("found %v targets", targets)
			if len(targets) > 0 {
				//de-dupe discovered targets
				job, sCfg := generateStaticPodConfig(pMon, ep, i, targets)
				if _, ok := cfgMap[job]; !ok {
					cfgMap[job] = sCfg
				}
			}
		}
	}

	return cfgMap, nil
}

func generateStaticPodConfig(
	m *promoperatorv1.PodMonitor,
	ep promoperatorv1.PodMetricsEndpoint,
	i int,
	targets []string,
) (key string, out yaml.MapSlice) {
	cfg := promcfg.ScrapeConfig{
		JobName:                 fmt.Sprintf("podMonitor/%s/%s/%d", m.Namespace, m.Name, i),
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

	// cfg.RelabelConfigs = append(cfg.RelabelConfigs, relabelServiceMonitor(m.Spec)...)

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
	return cfg.JobName, ret
}
