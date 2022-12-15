package v1

import (
	"net/mail"
	"net/url"
	"strings"

	promql "github.com/prometheus/prometheus/promql/parser"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/validation"
	"golang.org/x/exp/slices"
)

func validComparionOperator(op string) error {
	if !slices.Contains(shared.ComparisonOperators, op) {
		return validation.Error("Invalid comparison operator")
	}
	return nil
}

func (s *SilenceRequest) Validate() error {
	if err := s.ConditionId.Validate(); err != nil {
		return err
	}
	if s.Duration == nil || s.Duration.AsDuration() == 0 {
		return validation.Error("Require a non-zero Duration to activate a silence")
	}
	return nil
}

func (s *AlertConditionSystem) Validate() error {
	return nil
}

func (k *AlertConditionKubeState) Validate() error {
	if k.ClusterId == "" {
		return validation.Error("clusterId must be set")
	}
	if k.ObjectType == "" {
		return validation.Error("objectType must be set")
	}
	if k.ObjectName == "" {
		return validation.Error("objectName must be set")
	}
	if k.Namespace == "" {
		return validation.Error("objectNamespace must be set")
	}
	if !slices.Contains(shared.KubeStates, k.State) {
		return validation.Errorf("state must be one of the following: %v", shared.KubeStates)
	}
	return nil
}

func (c *AlertConditionComposition) Validate() error {
	return shared.WithUnimplementedErrorf("Composition alerts not implemented yet")
}

func (c *AlertConditionControlFlow) Validate() error {
	return shared.WithUnimplementedErrorf("Control flow alerts not implemented yet")
}

func (c *AlertConditionCPUSaturation) Validate() error {
	if c.ClusterId.Id == "" {
		return validation.Error("clusterId must be set")
	}
	if !(c.ExpectedRatio >= 0 && c.ExpectedRatio <= 1) {
		return validation.Error("expectedRatio must be between 0 and 1")
	}
	if c.For.AsDuration() == 0 {
		return validation.Error("\"for\" duration must be set")
	}
	if len(c.CpuStates) == 0 {
		return validation.Error("At least one usage type should be set")
	}
	return validComparionOperator(c.Operation)
}

func (c *AlertConditionMemorySaturation) Validate() error {
	if c.ClusterId.Id == "" {
		return validation.Error("clusterId must be set")
	}
	if !(c.ExpectedRatio >= 0 && c.ExpectedRatio <= 1) {
		return validation.Error("expectedRatio must be between 0 and 1")
	}
	if c.For.AsDuration() == 0 {
		return validation.Error("\"for\" duration must be set")
	}
	if len(c.UsageTypes) == 0 {
		return validation.Error("At least one usage type should be set")
	}
	return validComparionOperator(c.Operation)
}

func (f *AlertConditionFilesystemSaturation) Validate() error {
	if f.ClusterId.Id == "" {
		return validation.Error("clusterId must be set")
	}
	if !(f.ExpectedRatio >= 0 && f.ExpectedRatio <= 1) {
		return validation.Error("expectedRatio must be between 0 and 1")
	}
	if f.For.AsDuration() == 0 {
		return validation.Error("\"for\" duration must be set")
	}
	return validComparionOperator(f.Operation)
}

func (q *AlertConditionPrometheusQuery) Validate() error {
	if q.ClusterId.Id == "" {
		return validation.Error("clusterId must be set")
	}
	if q.Query == "" {
		return validation.Error("Prometheus query must be non-empty")
	}
	if _, err := promql.ParseExpr(q.Query); err != nil {
		return validation.Errorf("Invalid prometheus query : %s ", err)
	}
	if q.For.AsDuration() == 0 {
		return validation.Error("\"for\" duration must be set")
	}
	return nil
}

func (dc *AlertConditionDownstreamCapability) Validate() error {
	if dc.ClusterId.Id == "" {
		return validation.Error("clusterId must be set")
	}
	if dc.For.AsDuration() <= 0 {
		return validation.Error("positive \"for\" duration must be set")
	}
	if len(dc.CapabilityState) == 0 {
		return validation.Error("At least one bad capability state required for alerting")
	}
	return nil
}

func (m *AlertConditionMonitoringBackend) Validate() error {
	if m.For.AsDuration() == 0 {
		return validation.Error("\"for\" duration must be some positive time")
	}
	if len(m.BackendComponents) == 0 {
		return validation.Error("At least one backend component must be set to track")
	}
	return nil
}

func (d *AlertTypeDetails) Validate() error {
	if d.GetSystem() != nil {
		return d.GetSystem().Validate()
	}
	if d.GetDownstreamCapability() != nil {
		return d.GetDownstreamCapability().Validate()
	}
	if d.GetKubeState() != nil {
		return d.GetKubeState().Validate()
	}
	if d.GetCpu() != nil {
		return d.GetCpu().Validate()
	}
	if d.GetMemory() != nil {
		return d.GetMemory().Validate()
	}
	if d.GetFs() != nil {
		return d.GetFs().Validate()
	}
	if d.GetPrometheusQuery() != nil {
		return d.GetPrometheusQuery().Validate()
	}
	if d.GetComposition() != nil {
		return d.GetComposition().Validate()
	}
	if d.GetControlFlow() != nil {
		return d.GetControlFlow().Validate()
	}
	if d.GetMonitoringBackend() != nil {
		return d.GetMonitoringBackend().Validate()
	}
	return validation.Errorf("Backend does not handle alert type provided %v", d)
}

func (a *AlertCondition) Validate() error {
	if a.GetName() == "" {
		return validation.Error("AlertCondition name must be set")
	}
	if err := a.GetAlertType().Validate(); err != nil {
		return err
	}
	if a.AttachedEndpoints != nil {
		if err := a.AttachedEndpoints.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (e *EndpointImplementation) Validate() error {
	if e.Title == "" {
		return validation.Error("Title must be set")
	}
	if e.Body == "" {
		return validation.Error("Body must be set")
	}
	return nil
}

func (l *ListAlertConditionRequest) Validate() error {
	return nil
}

func (u *UpdateAlertConditionRequest) Validate() error {
	if u.Id == nil {
		return validation.Error("Id must be set")
	}
	if u.Id.Id == "" {
		return validation.Error("Id must be set")
	}
	if err := u.GetUpdateAlert().Validate(); err != nil {
		return err
	}
	return nil
}

func (a *AlertDetailChoicesRequest) Validate() error {
	return nil
}

func (a *AlertEndpoint) Validate() error {
	if a.GetName() == "" {
		return validation.Error("AlertEndpoint name must be set")
	}
	if a.GetSlack() != nil {
		return a.GetSlack().Validate()
	}
	if a.GetEmail() != nil {
		return a.GetEmail().Validate()
	}
	if a.GetPagerDuty() != nil {
		return a.GetPagerDuty().Validate()
	}
	return shared.WithUnimplementedErrorf("AlertEndpoint type %v not implemented yet", a)
}

func (s *SlackEndpoint) Validate() error {
	if s.WebhookUrl == "" {
		return validation.Error("webhook must be set")
	}
	// validate the url
	_, err := url.ParseRequestURI(s.WebhookUrl)
	if err != nil {
		return validation.Errorf("webhook must be a valid url : %s", err)
	}

	if s.Channel == "" {
		return validation.Error("channel must be set")
	}
	if !strings.HasPrefix(s.Channel, "#") {
		return validation.Error(shared.AlertingErrInvalidSlackChannel.Error())
	}
	return nil
}

func (e *EmailEndpoint) Validate() error {
	if e.To == "" {
		return validation.Error("email recipient must be set")
	}
	_, err := mail.ParseAddress(e.To)
	if err != nil {
		return validation.Errorf("Invalid recipient email %s", err)
	}
	if e.SmtpFrom != nil && *e.SmtpFrom != "" {
		_, err = mail.ParseAddress(*e.SmtpFrom)
		if err != nil {
			return validation.Errorf("Invalid sender email %s", err)
		}
	}
	if e.SmtpSmartHost != nil {
		arr := strings.Split(*e.SmtpSmartHost, ":")
		if len(arr) != 2 {
			return validation.Errorf("SMTP smart host must be in the form <address>:<port>, but got : %s", *e.SmtpSmartHost)
		}
	}
	return nil
}

func (a *PagerDutyEndpoint) Validate() error {
	if a.GetIntegrationKey() == "" {
		return validation.Error("integration key must be set for pager duty endpoint")
	}
	return nil
}

func (l *ListAlertEndpointsRequest) Validate() error {
	return nil
}

func (u *UpdateAlertEndpointRequest) Validate() error {
	if err := u.GetUpdateAlert().Validate(); err != nil {
		return err
	}
	return nil
}

func (c *RoutingNode) Validate() error {
	if c.ConditionId == nil {
		return validation.Error("ConditionId must be set")
	}
	if c.FullAttachedEndpoints == nil {
		return validation.Error("FullAttachedEndpoints must be set")
	}
	if err := c.FullAttachedEndpoints.Validate(); err != nil {
		return err
	}
	return nil
}

func (t *TestAlertEndpointRequest) Validate() error {
	if err := t.GetEndpoint().Validate(); err != nil {
		return err
	}
	return nil
}

func (f *FullAttachedEndpoints) Validate() error {
	if f.Details == nil {
		return validation.Error("Details must be set")
	}
	if err := f.GetDetails().Validate(); err != nil {
		return err
	}
	cache := map[string]struct{}{}
	for _, endpoint := range f.GetItems() {
		if _, ok := cache[endpoint.EndpointId]; ok {
			return validation.Errorf("Duplicate endpoint %s", endpoint.EndpointId)
		}
		cache[endpoint.EndpointId] = struct{}{}
		if err := endpoint.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (f *FullAttachedEndpoint) Validate() error {
	if f.EndpointId == "" {
		return validation.Error("EndpointId must be set")
	}
	if f.Details == nil {
		return validation.Error("Details must be set")
	}
	if err := f.GetDetails().Validate(); err != nil {
		return err
	}
	if f.AlertEndpoint == nil {
		return validation.Error("AlertEndpoint must be set")
	}
	if err := f.GetAlertEndpoint().Validate(); err != nil {
		return err
	}

	return nil
}

func (a *AttachedEndpoint) Validate() error {
	if a.EndpointId == "" {
		return validation.Error("attachedEndpoint endpoint id must be set")
	}
	return nil
}

func (a *AttachedEndpoints) Validate() error {
	if a.Details == nil {
		return validation.Error("attachedEndpoints details must be set")
	}
	if err := a.GetDetails().Validate(); err != nil {
		return err
	}
	cache := map[string]struct{}{}
	for _, item := range a.GetItems() {
		if _, ok := cache[item.EndpointId]; ok {
			return validation.Error("duplicate endpoint id in request")
		}
		cache[item.EndpointId] = struct{}{}
		if err := item.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (t *TimelineRequest) Validate() error {
	if t.GetLookbackWindow() == nil {
		return validation.Error("lookbackWindow must be set")
	}
	if t.GetLookbackWindow().GetSeconds() == 0 {
		return validation.Error("lookbackWindow must have a non zero time")
	}
	return nil
}

func (c *CloneToRequest) Validate() error {
	if len(c.GetToClusters()) == 0 {
		return validation.Error("toClusters must have a least one cluster set")
	}
	if err := c.GetAlertCondition().Validate(); err != nil {
		return err
	}
	return nil
}

func (e *EphemeralDispatcherRequest) Validate() error {
	if e.GetPrefix() == "" {
		return validation.Error("prefix must be set for ephemeral dispatchers")
	}
	if e.GetNumDispatches() <= 0 {
		return validation.Error("numDispatches must be non-zero for ephemeral dispatchers")
	}
	if e.GetTtl().AsDuration() == 0 {
		return validation.Error("numDuration must be non-zero for ephemeral dispatchers")
	}
	if err := e.GetDetails().Validate(); err != nil {
		return validation.Errorf("details must be valid for ephemeral dispatchers: %s", err)
	}
	if err := e.GetEndpoint().Validate(); err != nil {
		return validation.Errorf("endpoints must be valid for ephemeral dispatchers: %s", err)
	}
	return nil
}
