package discovery

import (
	"context"
	"fmt"

	promcfg "github.com/prometheus/prometheus/config"

	"github.com/prometheus/prometheus/discovery"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"

	promoperatorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	promcommon "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type serviceMonitorScrapeConfigRetriever struct {
	client client.Client
	logger *zap.SugaredLogger
	// current namespace the collector is defined in.
	// it should only discoverer secrets for TLS/auth in this namespace.
	namespace string
}

func NewServiceMonitorScrapeConfigRetriever(
	logger *zap.SugaredLogger,
	client client.Client,
	namespace string,
) ScrapeConfigRetriever {
	return &serviceMonitorScrapeConfigRetriever{
		client:    client,
		logger:    logger,
		namespace: namespace,
	}
}

func (s *serviceMonitorScrapeConfigRetriever) Name() string {
	return "serviceMonitor"
}

func (s *serviceMonitorScrapeConfigRetriever) findServiceMonitors() (map[string]*promoperatorv1.ServiceMonitor, error) {
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
	return lo.Associate(serviceMonitorList.Items, func(item *promoperatorv1.ServiceMonitor) (string, *promoperatorv1.ServiceMonitor) {
		return item.ObjectMeta.Name + "-" + item.ObjectMeta.Namespace, item
	}), nil
}

func (s *serviceMonitorScrapeConfigRetriever) findEndpoints(svcMon *promoperatorv1.ServiceMonitor) (*discoveryv1.EndpointSliceList, error) {
	lg := s.logger.With(
		"serviceMonitor", svcMon.Namespace+"-"+svcMon.Name,
		"selector", svcMon.Spec.Selector,
		"nsSelector", svcMon.Spec.NamespaceSelector,
	)
	selectorMap, err := metav1.LabelSelectorAsMap(&svcMon.Spec.Selector)
	if err != nil {
		return nil, err
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
			return nil, err
		}
		endpList.Items = append(endpList.Items, eList.Items...)
	} else {
		ns := &corev1.NamespaceList{}
		err := s.client.List(context.TODO(), ns)
		if err != nil {
			return nil, err
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
				lg.Warnf("failed to select endpointslices for service monitor %s: %s", err, selectorMap)
				continue
			}
			endpList.Items = append(endpList.Items, eList.Items...)
		}
	}
	return endpList, nil
}

func (s *serviceMonitorScrapeConfigRetriever) resolveEndpSliceTargets(
	svcMon promoperatorv1.ServiceMonitor,
	endpSlice discoveryv1.EndpointSlice,
	ep promoperatorv1.Endpoint,
) (targets []target) {
	targetPorts := []int32{}
	for _, port := range endpSlice.Ports {
		if port.Port == nil {
			// TODO
			// this indicates "all ports", we can't handle this in our static config
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
				// targets = append(targets, fmt.Sprintf("%s:%d", addr, targetPort))
				targets = append(targets, target{
					staticAddress: fmt.Sprintf("%s:%d", addr, targetPort),
					friendlyName:  s.generateFriendlyJobName(svcMon, endpSlice),
				})
			}
		}
	}
	return targets
}

func (s *serviceMonitorScrapeConfigRetriever) findServices(svcMon *promoperatorv1.ServiceMonitor) (*corev1.ServiceList, error) {
	lg := s.logger.With(
		"serviceMonitor", svcMon.Namespace+"-"+svcMon.Name,
		"selector", svcMon.Spec.Selector,
		"nsSelector", svcMon.Spec.NamespaceSelector,
	)
	selectorMap, err := metav1.LabelSelectorAsMap(&svcMon.Spec.Selector)
	if err != nil {
		return nil, err
	}
	lg = lg.With("selector", svcMon.Spec.Selector)
	lg = lg.With("nsSelector", svcMon.Spec.NamespaceSelector)
	nSel := svcMon.Spec.NamespaceSelector
	svcList := &corev1.ServiceList{}
	if nSel.Any || len(nSel.MatchNames) == 0 {
		sList := &corev1.ServiceList{}
		listOptions := &client.ListOptions{
			Namespace:     metav1.NamespaceAll,
			LabelSelector: labels.SelectorFromSet(selectorMap),
		}
		err = s.client.List(context.TODO(), sList, listOptions)
		if err != nil {
			return nil, err
		}
		svcList.Items = append(svcList.Items, sList.Items...)
	} else {
		ns := &corev1.NamespaceList{}
		err := s.client.List(context.TODO(), ns)
		if err != nil {
			return nil, err
		}
		toMatch := []string{}
		for _, n := range ns.Items {
			if slices.Contains(nSel.MatchNames, n.Name) {
				toMatch = append(toMatch, n.Name)
			}
		}
		for _, ns := range toMatch {
			sList := &corev1.ServiceList{}
			listOptions := &client.ListOptions{
				Namespace:     ns,
				LabelSelector: labels.SelectorFromSet(selectorMap),
			}
			err = s.client.List(context.TODO(), sList, listOptions)
			if err != nil {
				lg.Warnf("failed to select endpointslices for service monitor %s: %s", err, selectorMap)
				continue
			}
			svcList.Items = append(svcList.Items, sList.Items...)
		}
	}
	return svcList, nil
}

func (s *serviceMonitorScrapeConfigRetriever) generateStaticAddress(
	pod corev1.Pod, port corev1.ContainerPort,
) string {
	return fmt.Sprintf("%s:%d", pod.Status.PodIP, port.ContainerPort)
}

func (s *serviceMonitorScrapeConfigRetriever) resolveServiceTargets(
	svcMon promoperatorv1.ServiceMonitor,
	svc corev1.Service,
	ep promoperatorv1.Endpoint,
) (targets []target) {
	lg := s.logger.With(
		"serviceMonitor", svcMon.Namespace+"-"+svcMon.Name,
		"service", svc.Namespace+"-"+svc.Name,
	)
	podList := &corev1.PodList{}
	listOptions := &client.ListOptions{
		Namespace:     svc.Namespace,
		LabelSelector: labels.SelectorFromSet(svc.Labels),
	}
	err := s.client.List(context.TODO(), podList, listOptions)
	if err != nil {
		lg.Warnf("failed to find pods for service %s", svc.Namespace+"-"+svc.Name)
		return
	}
	lg.Debugf("found %d pods matching service", len(podList.Items))
	// deref pods
	for _, pod := range podList.Items {
		for _, container := range pod.Spec.Containers {
			for _, port := range container.Ports {
				if ep.Port != "" && port.Name != "" {
					// lg.Debugf("trying to match port name %s", port.Name)
					if ep.Port == port.Name {
						// lg.Debugf("matched port name %s", port.Name)
						targets = append(targets, target{
							staticAddress: s.generateStaticAddress(pod, port),
							friendlyName:  s.generateFriendlySvcJobName(svcMon, svc),
						})
						continue
					}
				}
				if ep.TargetPort != nil {
					switch ep.TargetPort.Type {
					case intstr.Int:
						// lg.Debugf("trying to match target port number %d", ep.TargetPort.IntVal)
						if port.ContainerPort == ep.TargetPort.IntVal {
							// lg.Debug("matched target port number")
							targets = append(targets, target{
								staticAddress: s.generateStaticAddress(pod, port),
								friendlyName:  s.generateFriendlySvcJobName(svcMon, svc),
							})
						}
					case intstr.String:
						// lg.Debugf("trying to match target port number %s", ep.TargetPort.StrVal)
						if port.Name != "" && port.Name == ep.TargetPort.StrVal {
							// lg.Debug("matched target port name")
							targets = append(targets, target{
								staticAddress: s.generateStaticAddress(pod, port),
								friendlyName:  s.generateFriendlySvcJobName(svcMon, svc),
							})
						}
					}
				}
			}
		}
	}
	if len(targets) == 0 { // try to find using endpoints
		lg.Info("no matching pods for service monitor, trying direct endpoint loookup")
		endpointList := &corev1.EndpointsList{}
		err := s.client.List(context.TODO(), endpointList, listOptions)
		if err != nil {
			lg.Warnf("failed to find pods for service %s", svc.Namespace+"-"+svc.Name)
			return
		}
		// lg.Debugf("found %d endpoints matching service", len(endpointList.Items))
		for _, endp := range endpointList.Items {
			for _, subset := range endp.Subsets {
				for _, port := range subset.Ports {
					if ep.Port != "" && port.Name != "" {
						// lg.Debugf("trying to match port name %s", port.Name)
						if ep.Port == port.Name {
							// lg.Debugf("matched port name %s", port.Name)
							for _, addr := range subset.Addresses {
								targets = append(targets, target{
									staticAddress: fmt.Sprintf("%s:%d", addr.IP, port.Port),
									friendlyName:  s.generateFriendlySvcJobName(svcMon, svc),
								})
							}
							continue
						}
					}
					if ep.TargetPort != nil {
						switch ep.TargetPort.Type {
						case intstr.Int:
							// lg.Debugf("trying to match target port number %d", ep.TargetPort.IntVal)
							if port.Port == ep.TargetPort.IntVal {
								// lg.Debug("matched target port number")
								for _, addr := range subset.Addresses {
									targets = append(targets, target{
										staticAddress: fmt.Sprintf("%s:%d", addr.IP, port.Port),
										friendlyName:  s.generateFriendlySvcJobName(svcMon, svc),
									})
								}
							}
						case intstr.String:
							// lg.Debugf("trying to match target port number %s", ep.TargetPort.StrVal)
							if port.Name != "" && port.Name == ep.TargetPort.StrVal {
								// lg.Debug("matched target port name")
								for _, addr := range subset.Addresses {
									targets = append(targets, target{
										staticAddress: fmt.Sprintf("%s:%d", addr.IP, port.Port),
										friendlyName:  s.generateFriendlySvcJobName(svcMon, svc),
									})
								}
							}
						}
					}
				}
			}
		}
	}

	return
}

func (s serviceMonitorScrapeConfigRetriever) Yield() (cfg *promCRDOperatorConfig, retErr error) {
	sMons, err := s.findServiceMonitors()
	if err != nil {
		return nil, err
	}

	// jobName -> scrapeConfig
	cfgMap := jobs{}
	secretRes := []SecretResolutionConfig{}
	s.logger.Debugf("found %d service monitors", len(sMons))
	for _, svcMon := range sMons {
		lg := s.logger.With(
			"serviceMonitor", svcMon.Namespace+"-"+svcMon.Name,
			"selector", svcMon.Spec.Selector,
			"nsSelector", svcMon.Spec.NamespaceSelector,
		)
		numTargets := 0
		svcList, err := s.findServices(svcMon)
		if err != nil {
			lg.Warnf("failed to select services for service monitor %s: %s", svcMon.Spec.Selector, err)
			continue
		}
		lg.Debugf("found %d matching services", len(svcList.Items))
		for _, svc := range svcList.Items {
			for i, ep := range svcMon.Spec.Endpoints {
				targets := s.resolveServiceTargets(*svcMon, svc, ep)
				numTargets += len(targets)
				if len(targets) > 0 {
					job, sCfg, secrets := s.generateStaticServiceConfig(svcMon, ep, i, targets)
					if _, ok := cfgMap[job]; !ok {
						cfgMap[job] = sCfg
					}
					secretRes = append(secretRes, secrets...)
				}
			}
		}

		if numTargets == 0 {
			lg.Warn("no scrape targets found for service monitor")
		}

		// endpList, err := s.findEndpoints(svcMon)
		// if err != nil {
		// 	lg.Warnf("failed to select endpointslices for service monitor %s: %s", err, svcMon.Spec.Selector)
		// 	continue
		// }
		// lg.Debugf("found %d matching endpoint slices", len(endpList.Items))
		// for _, endpSlice := range endpList.Items {
		// 	lg = lg.With("endpSlice", endpSlice.Namespace+"-"+endpSlice.Name)
		// 	// see if we can match any endpoints
		// 	for i, ep := range svcMon.Spec.Endpoints {
		// 		targets := s.resolveEndpSliceTargets(*svcMon, endpSlice, ep)
		// 		// TODO : we need to deref pods and see if additional ports exist there
		// 		// TODO : there is an edge case where we can skip checking pods; if all endpoints
		// 		// are found on the endpoint slice
		// 		if len(targets) > 0 {
		// 			job, sCfg, secrets := s.generateStaticServiceConfig(svcMon, ep, i, targets, endpSlice)
		// 			if _, ok := cfgMap[job]; !ok {
		// 				cfgMap[job] = sCfg
		// 			}
		// 			secretRes = append(secretRes, secrets...)
		// 		}
		// 		lg.Debugf("found %d targets for endpoint : %v", len(targets), targets)
		// 	}
		// }
	}
	return &promCRDOperatorConfig{
		jobs:    cfgMap,
		secrets: secretRes,
	}, nil
}

var _ ScrapeConfigRetriever = &serviceMonitorScrapeConfigRetriever{}

func (s *serviceMonitorScrapeConfigRetriever) generateStaticServiceConfig(
	m *promoperatorv1.ServiceMonitor,
	ep promoperatorv1.Endpoint,
	i int,
	targets []target,
) (key string, out yaml.MapSlice, secretRes []SecretResolutionConfig) {
	uid := fmt.Sprintf("serviceMonitor/%s/%s/%d", m.Namespace, m.Name, i)
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
	} else {
		cfg.ScrapeInterval = model.Duration(opniDefaultScrapeInterval)
	}
	if ep.ScrapeTimeout != "" {
		dur := parseStrToDuration(string(ep.ScrapeTimeout))
		cfg.ScrapeTimeout = dur
	} else {
		cfg.ScrapeTimeout = model.Duration(opniDefaultScrapeTimeout)
	}
	if ep.Path != "" {
		cfg.MetricsPath = ep.Path
	}
	if ep.Scheme != "" {
		cfg.Scheme = ep.Scheme
	} else {
		cfg.Scheme = "http"
	}

	// handle spec relabelling configs
	cfg.RelabelConfigs = append(cfg.RelabelConfigs, append(jobRelabelling(targets[0].friendlyName), generateRelabelConfig(ep.RelabelConfigs)...)...)
	cfg.MetricRelabelConfigs = append(cfg.MetricRelabelConfigs, generateRelabelConfig(ep.MetricRelabelConfigs)...)

	// handle basic network auth config conversion
	if ep.BearerTokenFile != "" {
		cfg.HTTPClientConfig.Authorization = &promcommon.Authorization{
			Type:            "Bearer",
			CredentialsFile: ep.BearerTokenFile,
		}
	}

	if ep.BearerTokenSecret.Name != "" {
		bearerSecret := &corev1.Secret{}
		err := s.client.Get(context.TODO(), types.NamespacedName{
			Name:      ep.BearerTokenSecret.Name,
			Namespace: s.namespace,
		}, bearerSecret)
		if err == nil {
			bearerSecretRes := SecretResolutionConfig{
				TargetKey: ep.BearerTokenSecret.Key,
				namespace: s.namespace,
				Secret:    bearerSecret,
			}
			secretRes = append(secretRes, bearerSecretRes)
			cfg.HTTPClientConfig.Authorization = &promcommon.Authorization{
				Type:            "Bearer",
				CredentialsFile: bearerSecretRes.Path(),
			}
		} else {
			s.logger.Warnf("failed to find a specified bearer token secret : %v", ep.BearerTokenSecret)
		}
	}

	if cfg.HTTPClientConfig.Authorization == nil {
		if ep.Authorization != nil {
			authorizationCfg, secrets := fromSafeAuthorization(
				s.client,
				ep.Authorization,
				s.logger,
				s.namespace,
			)
			secretRes = append(secretRes, secrets...)
			cfg.HTTPClientConfig.Authorization = &authorizationCfg
		}
	}

	if ep.TLSConfig != nil {
		if !isEmpty(ep.TLSConfig.SafeTLSConfig) {
			tlsCfg, secrets := fromSafeTlsConfig(
				s.client,
				ep.TLSConfig.SafeTLSConfig,
				s.logger,
				s.namespace,
			)
			secretRes = append(secretRes, secrets...)
			cfg.HTTPClientConfig.TLSConfig = tlsCfg
		} else {
			cfg.HTTPClientConfig.TLSConfig = promcommon.TLSConfig{
				CAFile:             ep.TLSConfig.CAFile,
				CertFile:           ep.TLSConfig.CertFile,
				KeyFile:            ep.TLSConfig.KeyFile,
				ServerName:         ep.TLSConfig.ServerName,
				InsecureSkipVerify: ep.TLSConfig.InsecureSkipVerify,
			}
		}
	}

	// handle conversion to explicit scrape targets
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
			Key: "targets",
			Value: lo.Map(targets, func(t target, _ int) string {
				return t.staticAddress
			}),
		},
	}

	// we need to add operator specific labels to the static config
	sdLabels := yaml.MapSlice{
		{
			Key: "labels",
			Value: yaml.MapSlice{
				{
					Key:   overrideJobName,
					Value: targets[0].friendlyName,
				},
			},
		},
	}

	staticConfig := yaml.MapItem{
		Key: "static_configs",
		Value: []yaml.MapSlice{
			sd,
			sdLabels,
		},
	}
	ret = append(ret, staticConfig)

	return uid, ret, secretRes
}

func (s *serviceMonitorScrapeConfigRetriever) generateFriendlyJobName(
	m promoperatorv1.ServiceMonitor,
	endpSlice discoveryv1.EndpointSlice,
) string {
	if m.Spec.JobLabel != "" {
		if val, ok := endpSlice.Labels[m.Spec.JobLabel]; ok {
			return val
		}
	}
	return endpSlice.Name
}

func (s *serviceMonitorScrapeConfigRetriever) generateFriendlySvcJobName(
	m promoperatorv1.ServiceMonitor,
	svc corev1.Service,
) string {
	if m.Spec.JobLabel != "" {
		if val, ok := svc.Labels[m.Spec.JobLabel]; ok {
			return val
		}
	}
	return svc.Name
}
