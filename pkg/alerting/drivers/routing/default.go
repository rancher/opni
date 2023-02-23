package routing

import (
	"net/url"
	"time"

	amCfg "github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/alerting/drivers/config"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
)

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// DefaultRouteValues (receiver name, rate limiting config)
type DefaultRouteValues lo.Tuple2[string, rateLimitingConfig]

func DefaultSubTreeLabel() string {
	return shared.OpniSeverityLabel
}

// ! values must be returned sorted in a deterministic order
//
// these are used by the router to catch eveyrthing that is not an explicit alarm
// that is also part of opni, i.e. plain text notifications
func DefaultSubTreeValues() []DefaultRouteValues {
	// sorted by ascending severity
	severityKey := lo.Keys(alertingv1.OpniSeverity_name)
	slices.SortFunc(severityKey, func(a, b int32) bool {
		return a < b
	})
	res := []DefaultRouteValues{}
	n := len(alertingv1.OpniSeverity_name)
	for i, sev := range severityKey {
		r := time.Duration(3*abs(i-n)) - 2 // r = formula for throttling factor
		res = append(res, DefaultRouteValues{
			A: alertingv1.OpniSeverity_name[sev],
			B: rateLimitingConfig{
				InitialDelay:       time.Second * 10,
				RepeatInterval:     time.Hour * 5,
				ThrottlingDuration: r * time.Minute,
			},
		})
	}
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

func NewOpniSeverityLabels(title, body, severity string) (map[string]string, error) {
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
				URL: &amCfg.URL{
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
	defaultNamespaceRoute, recvs := NewOpniNamespacedSubTree(DefaultSubTreeLabel(), DefaultSubTreeValues()...)

	// rate limits messages pushed from an opni source that itself should decide how to group messages
	defaultNamespaceRoute.GroupBy = append(defaultNamespaceRoute.GroupBy, shared.OpniUnbufferedKey)
	opniRoute.Routes = append(opniRoute.Routes, defaultNamespaceRoute)
	allRecvs = append(allRecvs, recvs...)

	// must be last to prevent any opni alerts from leaking into the user's production routing tree
	opniRoute.Routes = append(opniRoute.Routes, newFinalizer(nil))
	return opniRoute, allRecvs
}

func newNamespaceParentMatcher(namespace string) *labels.Matcher {
	return &labels.Matcher{
		Name:  namespace,
		Value: "",
		Type:  labels.MatchNotEqual,
	}
}

func newFinalizer(optionalNamespace *string) *config.Route {
	matchers := func() []*labels.Matcher {
		if optionalNamespace == nil {
			return []*labels.Matcher{}
		}
		return []*labels.Matcher{
			newNamespaceParentMatcher(*optionalNamespace),
		}
	}

	finalizerRoute := &config.Route{
		Matchers: matchers(),
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

func setRateLimiting(route *config.Route, rl rateLimitingConfig) {
	route.RepeatInterval = lo.ToPtr(model.Duration(rl.RepeatInterval))
	route.GroupWait = lo.ToPtr(model.Duration(rl.InitialDelay))
	route.GroupInterval = lo.ToPtr(model.Duration(rl.ThrottlingDuration))
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

func NewOpniNamespacedSubTree(namespace string, defaultValues ...DefaultRouteValues) (*config.Route, []*config.Receiver) {
	parentRoute := &config.Route{
		GroupBy: []model.LabelName{"..."},
		Matchers: config.Matchers{
			newNamespaceParentMatcher(namespace),
		},
		// namespaces should be isolated, so finalizers are the ones that have continue = False
		// continue = false here overrides the subtree's continue = true?
		Continue:       false,
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
	finalizer := newFinalizer(lo.ToPtr(namespace))
	subRoutes = append(subRoutes, finalizer) // finalizer must always be last
	parentRoute.Routes = subRoutes
	return parentRoute, receivers
}

func NewOpniSubRoutingTreeWithDefaultValue(namespace string, value DefaultRouteValues) (*config.Route, *config.Receiver) {
	valueRoute := &config.Route{
		Matchers: config.Matchers{
			newNamespaceMatcher(namespace, value.A),
		},
		// even though each value in the namespace should be unique, we should traverse the entire namespace
		// until we make it to the finalizer route
		Continue: true,
	}
	setRateLimiting(valueRoute, value.B)
	constructedReceiverId := shared.NewOpniReceiverName(shared.OpniReceiverId{
		Namespace:  namespace,
		ReceiverId: value.A,
	})
	valueRoute.Receiver = constructedReceiverId
	valueReceiver := &config.Receiver{
		Name: constructedReceiverId,
	}
	return valueRoute, valueReceiver
}

// new namespace matcher
func NewOpniSubRoutingTreeWithValue(rl rateLimitingConfig, opniConfigs []config.OpniReceiver, matchers []*labels.Matcher, receiverId string) (*config.Route, *config.Receiver) {
	valueRoute := &config.Route{
		Matchers: matchers,
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
	valueRoute.Receiver = receiverId
	valueReceiver, err := config.BuildReceiver(receiverId, opniConfigs)
	if err != nil {
		panic(err)
	}
	return valueRoute, valueReceiver
}
