package routing

import (
	"net/url"
	"time"

	"github.com/prometheus/alertmanager/pkg/labels"
	promcfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/alerting/drivers/config"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
)

func DefaultSubTreeLabel() string {
	return shared.OpniSeverityLabel
}

// ! values must be returned sorted in a deterministic order
func DefaultSubTreeValues() []string {
	res := lo.Values(alertingv1.OpniSeverity_name)
	slices.SortFunc(res, func(a, b string) bool {
		return a < b
	})
	return res
}

func NewOpniAlarmLabels(conditionId string) (map[string]string, error) {
	treeLabels, err := shared.AlertManagerLabelsToAnnotations(shared.OpniSubRoutingTreeMatcher)
	if err != nil {
		return nil, err
	}
	return lo.Assign(map[string]string{
		shared.BackendConditionIdLabel: conditionId,
	},
		treeLabels,
	), nil
}

func NewOpniSeverityLabels(key, title, body, severity string) (map[string]string, error) {
	treeLabels, err := shared.AlertManagerLabelsToAnnotations(shared.OpniSeverityTreeMatcher)
	if err != nil {
		return nil, err
	}
	return lo.Assign(
		map[string]string{
			shared.OpniSeverityLabel: severity,
			shared.OpniTitleLabel:    title,
			shared.OpniBodyLabel:     body,
		},
		treeLabels,
	), nil
}

var OpniSubRoutingTreeId config.Matchers = []*labels.Matcher{
	shared.OpniSubRoutingTreeMatcher,
}

func DefaultOpniReceiver(embeddedServerHook string) *config.Receiver {
	return &config.Receiver{
		Name: shared.AlertingHookReceiverName,
		WebhookConfigs: []*config.WebhookConfig{
			{
				URL: &promcfg.URL{
					URL: util.Must(url.Parse(embeddedServerHook)),
				},
			},
		},
	}
}

func NewDefaultRoutingTree(embeddedServerHook string) *config.Config {
	root := NewDefaultRoutingTreeRoot(embeddedServerHook)
	subtree, recvs := NewOpniSubRoutingTree()
	metricsSubtree := NewOpniMetricsSubtree()
	root.Route.Routes = append(root.Route.Routes, subtree)
	root.Route.Routes = append(root.Route.Routes, metricsSubtree)
	slices.SortFunc(recvs, func(a, b *config.Receiver) bool {
		return a.Name < b.Name
	})
	// !! finalizer route must always be last
	root.Receivers = append(recvs, root.Receivers...)
	return root
}

// This needs to expand all labels, due to assumptions we make about external backends like
// AiOps pushing messages to the severity tree
func NewDefaultRoutingTreeRoot(embeddedServerHook string) *config.Config {
	return &config.Config{
		Global: lo.ToPtr(config.DefaultGlobalConfig()),
		Route: &config.Route{
			Receiver: shared.AlertingHookReceiverName,
			// special character that expands all groups, need to expand labels so they can
			// be subgrouped to opni-specific subrouting trees and user-synced subrouting trees
			GroupBy:   []model.LabelName{"..."},
			Routes:    []*config.Route{},
			GroupWait: lo.ToPtr(model.Duration(60 * time.Second)),
			//GroupInterval:  lo.ToPtr(model.Duration(1 * time.Minute)),
			RepeatInterval: lo.ToPtr(model.Duration(5 * time.Hour)),
		},
		Receivers: []*config.Receiver{
			DefaultOpniReceiver(embeddedServerHook),
		},
		MuteTimeIntervals: []config.MuteTimeInterval{},
		InhibitRules:      []*config.InhibitRule{},
		Templates:         []string{},
	}
}

// returns the subtree & the default receivers
// contains the setup for broadcasting and conditions
func NewOpniSubRoutingTree() (*config.Route, []*config.Receiver) {
	opniRoute := &config.Route{
		GroupBy: shared.OpniGroupByClause,

		Matchers: OpniSubRoutingTreeId,
		Routes:   []*config.Route{},
		Continue: true, // we want to expand the sub trees
	}
	allRecvs := []*config.Receiver{}
	// default namespace is based on the grpc enum status
	defaultNamespaceRoute, recv := NewOpniNamespacedSubTree(DefaultSubTreeLabel(), DefaultSubTreeValues()...)

	// rate limits messages pushed from an opni source that itself should decide how to group messages
	defaultNamespaceRoute.GroupBy = append(defaultNamespaceRoute.GroupBy, shared.OpniUnbufferedKey)
	opniRoute.Routes = append(opniRoute.Routes, defaultNamespaceRoute)
	allRecvs = append(allRecvs, recv...)

	// must be last to prevent any opni alerts from leaking into the user's production routing tree
	opniRoute.Routes = append(opniRoute.Routes, newFinalizer())
	return opniRoute, allRecvs
}

func newNamespaceParentMatcher(namespace string) *labels.Matcher {
	return &labels.Matcher{
		Name:  namespace,
		Value: "",
		Type:  labels.MatchNotEqual,
	}
}

func newFinalizer() *config.Route {
	finalizerRoute := &config.Route{
		Receiver: shared.AlertingHookReceiverName, // assumption : always present in opni embedded server
		// set to false to prevent any further routing into unrelated namespace OR
		// the user's synced production config
		Continue: false,
	}
	return finalizerRoute
}

func newNamespaceMatcher(namespace, value string) *labels.Matcher {
	return &labels.Matcher{
		Name:  namespace,
		Value: value,
		Type:  labels.MatchEqual,
	}
}

func setDefaultRateLimitingFromProto(route *config.Route) {
	rateLimitingConfig := (&alertingv1.RateLimitingConfig{}).Default()
	if dur := rateLimitingConfig.GetThrottlingDuration(); dur != nil {
		modelDur := model.Duration(dur.AsDuration())
		route.GroupInterval = &modelDur
	} else {
		modelDur := model.Duration(time.Minute * 10)
		route.GroupInterval = &modelDur
	}
	if delay := rateLimitingConfig.GetInitialDelay(); delay != nil {
		dur := model.Duration(delay.AsDuration())
		route.GroupWait = &dur
	} else {
		dur := model.Duration(time.Second * 10)
		route.GroupWait = &dur
	}
	if rInterval := rateLimitingConfig.GetRepeatInterval(); rInterval != nil {
		dur := model.Duration(rInterval.AsDuration())
		route.RepeatInterval = &dur
	} else {
		dur := model.Duration(time.Minute * 10)
		route.RepeatInterval = &dur
	}
}

func NewOpniNamespacedSubTree(namespace string, defaultValues ...string) (*config.Route, []*config.Receiver) {
	parentRoute := &config.Route{
		GroupBy: []model.LabelName{"..."},
		Matchers: config.Matchers{
			newNamespaceParentMatcher(namespace),
		},
		Continue:       false, // namespaces should be isolated
		Routes:         []*config.Route{},
		GroupWait:      lo.ToPtr(model.Duration(60 * time.Second)),
		RepeatInterval: lo.ToPtr(model.Duration(5 * time.Hour)),
	}
	receivers := []*config.Receiver{}
	subRoutes := []*config.Route{}
	for _, val := range defaultValues {
		subRoute, recv := NewOpniSubRoutingTreeWithDefaultValue(namespace, val)
		subRoutes = append(subRoutes, subRoute)
		receivers = append(receivers, recv)
	}
	// always terminate the namespace with a finalizer
	finalizer := newFinalizer()
	subRoutes = append(subRoutes, finalizer) // finalizer must always be last
	parentRoute.Routes = subRoutes
	return parentRoute, receivers
}

func NewOpniSubRoutingTreeWithDefaultValue(namespace, value string) (*config.Route, *config.Receiver) {
	valueRoute := &config.Route{
		Matchers: config.Matchers{
			newNamespaceMatcher(namespace, value),
		},
		// even though each value in the namespace should be unique, we should traverse the entire namespace
		// until we make it to the finalizer route
		Continue: true,
	}
	setDefaultRateLimitingFromProto(valueRoute)
	constructedReceiverId := shared.NewOpniReceiverName(shared.OpniReceiverId{
		Namespace:  namespace,
		ReceiverId: value,
	})
	valueRoute.Receiver = constructedReceiverId
	valueReceiver := &config.Receiver{
		Name: constructedReceiverId,
	}
	return valueRoute, valueReceiver
}

func NewOpniSubRoutingTreeWithValue(
	namespace,
	value string,
	rl rateLimitingConfig,
	opniConfigs []config.OpniReceiver,
) (*config.Route, *config.Receiver) {
	valueRoute := &config.Route{
		Matchers: config.Matchers{
			newNamespaceMatcher(namespace, value),
		},
		// even though in the current implementation, each namespace value should be unique,
		// we should traverse the entire namespace until we make it to the finalizer route
		Continue: true,
	}
	setDefaultRateLimitingFromProto(valueRoute)
	throttleDur := model.Duration(rl.ThrottlingDuration)
	valueRoute.GroupInterval = &throttleDur
	initialDelayDur := model.Duration(rl.InitialDelay)
	valueRoute.GroupWait = &initialDelayDur
	repeatIntervalDur := model.Duration(rl.RepeatInterval)
	valueRoute.RepeatInterval = &repeatIntervalDur
	constructedReceiverId := shared.NewOpniReceiverName(shared.OpniReceiverId{
		Namespace:  namespace,
		ReceiverId: value,
	})
	valueRoute.Receiver = constructedReceiverId
	valueReceiver, err := config.BuildReceiver(constructedReceiverId, opniConfigs)
	if err != nil {
		panic(err)
	}
	return valueRoute, valueReceiver
}
