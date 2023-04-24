package discovery

import (
	"context"
	"fmt"

	promcfg "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"

	promoperatorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type serviceMonitorScrapeConfigRetriever struct {
	client client.Client
	logger *zap.SugaredLogger
}

func NewServiceMonitorScrapeConfigRetriever(
	logger *zap.SugaredLogger,
	client client.Client,
) ScrapeConfigRetriever {

	return &serviceMonitorScrapeConfigRetriever{
		client: client,

		logger: logger,
	}
}

func (s *serviceMonitorScrapeConfigRetriever) Name() string {
	return "serviceMonitor"
}

func (s serviceMonitorScrapeConfigRetriever) Yield() (cfg jobs, retErr error) {
	listOptions := &client.ListOptions{
		Namespace: metav1.NamespaceAll,
	}
	serviceMonitorList := &promoperatorv1.ServiceMonitorList{}
	err := s.client.List(
		context.TODO(),
		serviceMonitorList,
		listOptions,
	)
	if err != nil {
		return nil, err
	}
	sMons := lo.Associate(serviceMonitorList.Items, func(item *promoperatorv1.ServiceMonitor) (string, *promoperatorv1.ServiceMonitor) {
		return item.ObjectMeta.Name + "-" + item.ObjectMeta.Namespace, item
	})

	// jobName -> scrapeConfig
	cfgMap := jobs{}
	s.logger.Debugf("found %d service monitors", len(sMons))
	for _, svcMon := range sMons {
		lg := s.logger.With("serviceMonitor", svcMon.Namespace+"-"+svcMon.Name)
		selectorMap, err := metav1.LabelSelectorAsMap(&svcMon.Spec.Selector)
		if err != nil {
			continue
		}
		lg = lg.With("selector", svcMon.Spec.Selector)
		lg = lg.With("nsSelector", svcMon.Spec.NamespaceSelector)
		nSel := svcMon.Spec.NamespaceSelector
		endpList := &discoveryv1.EndpointSliceList{}
		if nSel.Any || len(nSel.MatchNames) == 0 {
			eList := &discoveryv1.EndpointSliceList{}
			listOptions := &client.ListOptions{
				Namespace:     metav1.NamespaceAll,
				LabelSelector: labels.SelectorFromSet(selectorMap),
			}
			err = s.client.List(context.TODO(), eList, listOptions)
			if err != nil {
				s.logger.Warnf("failed to select services for service monitor %s: %s", err, selectorMap)
				continue
			}
			endpList.Items = append(endpList.Items, eList.Items...)
		} else {
			ns := &corev1.NamespaceList{}
			err := s.client.List(context.TODO(), ns)
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
				eList := &discoveryv1.EndpointSliceList{}
				listOptions := &client.ListOptions{
					Namespace:     ns,
					LabelSelector: labels.SelectorFromSet(selectorMap),
				}
				err = s.client.List(context.TODO(), eList, listOptions)
				if err != nil {
					s.logger.Warnf("failed to select endpointslices for service monitor %s: %s", err, selectorMap)
					continue
				}
				endpList.Items = append(endpList.Items, eList.Items...)
			}
		}
		lg.Debugf("found %d matching endpoint slices", len(endpList.Items))
		for _, endpSlice := range endpList.Items {
			lg = lg.With("endpSlice", endpSlice.Namespace+"-"+endpSlice.Name)

			// see if we can match any endpoints
			for i, ep := range svcMon.Spec.Endpoints {
				targetPorts := []int32{}
				targets := []string{}
				for _, port := range endpSlice.Ports {
					if port.Port == nil {
						// this indicates "all ports", can't handle this in our static config
						// construction yet
						continue
					}
					if ep.Port != "" && port.Name != nil {
						if ep.Port == *port.Name {
							targetPorts = append(targetPorts, *port.Port)
							continue
						}
					}
					if ep.TargetPort != nil {
						switch ep.TargetPort.Type {
						case intstr.Int:
							if port.Port != nil && ep.TargetPort.IntVal == *port.Port {
								targetPorts = append(targetPorts, *port.Port)
							}
						case intstr.String:
							if port.Name != nil && port.Port != nil && ep.TargetPort.StrVal == *port.Name {
								targetPorts = append(targetPorts, *port.Port)
							}
						}
					}
				}

				// targetPorts are necessarily defined for all endpoints in the endpoints slice
				for _, targetPort := range targetPorts {
					for _, endp := range endpSlice.Endpoints {
						for _, addr := range endp.Addresses {
							targets = append(targets, fmt.Sprintf("%s:%d", addr, targetPort))
						}
					}
				}
				lg.Debugf("found %d targets for endpoint : %v", len(targets), targets)

				if len(targets) > 0 {
					job, sCfg := generateStaticServiceConfig(svcMon, ep, i, targets)
					if _, ok := cfgMap[job]; !ok {
						cfgMap[job] = sCfg
					}
				}
			}
		}

	}
	return cfgMap, nil
}

var _ ScrapeConfigRetriever = &serviceMonitorScrapeConfigRetriever{}

func generateStaticServiceConfig(
	m *promoperatorv1.ServiceMonitor,
	ep promoperatorv1.Endpoint,
	i int,
	targets []string,
) (key string, out yaml.MapSlice) {
	cfg := promcfg.ScrapeConfig{
		JobName:                 fmt.Sprintf("serviceMonitor/%s/%s/%d", m.Namespace, m.Name, i),
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
