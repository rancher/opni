package metrics

import (
	"errors"
	"fmt"
	"text/template"
	"time"

	"github.com/prometheus/common/model"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/rancher/opni/pkg/slo/backend"
	"gopkg.in/yaml.v3"
)

var DefaultEvaluationInterval = time.Minute

const (
	SLOSLI                  = "slo:sli_error:ratio_rate"
	SLOObjective            = "slo:objective:ratio"
	SLOErrorBudget          = "slo:error_budget:ratio"
	SLOPeriod               = "slo:time_period:duration"
	SLOCurrentBurnRate      = "slo:current_burn_rate:ratio_rate"
	SLOPeriodBurnRate       = "slo:period_burn_rate:ratio_rate"
	SLOErrorBudgetRemaining = "slo:error_budget_remaining:ratio"
	SLOInfo                 = "opni_slo_info"
)

func QueryWithLabels(query string, labels backend.IdentificationLabels) string {
	return fmt.Sprintf(
		"%s{%s}",
		query,
		labels.ToLabels().ConstructPrometheus(),
	)
}

type PrometheusSLO struct {
	SLI MetricGenerator
}

type MetricGenerator interface {
	Id() string
	Expr() string
	Rule() (rulefmt.RuleNode, error)
}

type MWMBSLOGenerator interface {
	Windows() []prommodel.Duration
	SLI(measurementPeriod prommodel.Duration) MetricGenerator

	Objective() MetricGenerator
	ErrorBudget() MetricGenerator
	Period() MetricGenerator
	Info() MetricGenerator

	CurrentBurnRate() MetricGenerator
	PeriodBurnRate() MetricGenerator
	ErrorBudgetRemaining() MetricGenerator

	// TODO : unimplemented
	// TicketWindow() MetricGenerator
	// PageWindow() MetricGenerator
	PageAlert() MetricGenerator
	TicketAlert() MetricGenerator
}

type SLOGeneratorOptions struct {
	optimized bool
}

func (s *SLOGeneratorOptions) Apply(opts ...SLOGeneratorOption) {
	for _, opt := range opts {
		opt(s)
	}
}

func DefaultSLOGeneratorOptions() *SLOGeneratorOptions {
	return &SLOGeneratorOptions{
		optimized: true,
	}
}

type SLOGeneratorOption func(*SLOGeneratorOptions)

func WithOptimization(optimized bool) SLOGeneratorOption {
	return func(o *SLOGeneratorOptions) {
		o.optimized = optimized
	}
}

func NewSLOGenerator(slo backend.SLO, opts ...SLOGeneratorOption) (*SLOGeneratorImpl, error) {
	options := DefaultSLOGeneratorOptions()
	options.Apply(opts...)
	if err := slo.Validate(); err != nil {
		return nil, err
	}
	dur, err := prommodel.ParseDuration(slo.SloPeriod)
	if err != nil {
		return nil, err
	}
	goodQuery := fmt.Sprintf("%s{job=\"%s\", %s}", slo.GoodMetric, slo.Svc, slo.GoodEvents.ConstructPrometheus())
	var totalQuery string
	if len(slo.TotalEvents) == 0 {
		totalQuery = fmt.Sprintf("%s{job=\"%s\"}", slo.TotalMetric, slo.Svc)
	} else {
		totalQuery = fmt.Sprintf("%s{job=\"%s\", %s}", slo.TotalMetric, slo.Svc, slo.TotalEvents.ConstructPrometheus())
	}
	var windows *MWMBWindows
	switch slo.SloPeriod {
	case "28d", "30d": //fallback to google SRE production presets
		windows = GenerateMWMBWindows(
			prommodel.Duration(time.Minute * 5),
		)
	default: // use somewhat hacky normalization
		windows = GenerateMWMBWindows(
			prommodel.Duration(NormalizePeriodToBudgetingInterval(time.Duration(dur))),
		)
	}

	return &SLOGeneratorImpl{
		slo:        slo,
		options:    options,
		goodQuery:  goodQuery,
		totalQuery: totalQuery,
		period:     dur,
		windows:    windows,
	}, nil
}

type SLOGeneratorImpl struct {
	slo backend.SLO

	goodQuery  string
	totalQuery string
	period     prommodel.Duration

	windows *MWMBWindows

	options *SLOGeneratorOptions
}

var _ MWMBSLOGenerator = (*SLOGeneratorImpl)(nil)
var _ RuleGroupBuilder = (*SLOGeneratorImpl)(nil)

func (s *SLOGeneratorImpl) groupId() string {
	return "slo-mwmb"
}

func (s *SLOGeneratorImpl) AsRuleGroup() (rulefmt.RuleGroup, error) {
	rules, err := s.rules()
	if err != nil {
		return rulefmt.RuleGroup{}, err
	}
	return rulefmt.RuleGroup{
		Name:     s.slo.GetId(),
		Interval: prommodel.Duration(DefaultEvaluationInterval),
		Rules:    rules,
	}, nil
}

func (s *SLOGeneratorImpl) rules() ([]rulefmt.RuleNode, error) {
	ruleConstructor := []func() (rulefmt.RuleNode, error){
		s.Objective().Rule,
		s.ErrorBudget().Rule,
		s.Period().Rule,
		s.Info().Rule,
		s.CurrentBurnRate().Rule,
		s.PeriodBurnRate().Rule,
		s.ErrorBudgetRemaining().Rule,
	}

	for _, window := range append(s.Windows(), s.period) {
		sli := s.SLI(window)
		ruleConstructor = append(ruleConstructor, sli.Rule)
	}

	rules := []rulefmt.RuleNode{}
	var errs []error
	for _, constructor := range ruleConstructor {
		rule, err := constructor()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		rules = append(rules, rule)
	}
	return rules, errors.Join(errs...)
}

func (s *SLOGeneratorImpl) Windows() []prommodel.Duration {
	return s.windows.windowRangeDur()
}

func (s *SLOGeneratorImpl) SLI(measurementPeriod prommodel.Duration) MetricGenerator {
	return NewMWMBSLIGenerator(
		SLOSLI,
		s.goodQuery,
		s.totalQuery,
		measurementPeriod,
		s.slo.IdLabels,
	)
}

func normalizeObjective(objective float64) float64 {
	return objective / 100
}

// ==== constants
func (s *SLOGeneratorImpl) Objective() MetricGenerator {
	return NewConstantMetricGenerator(SLOObjective, normalizeObjective(s.slo.Objective), 9, s.slo.IdLabels)
}

func (s *SLOGeneratorImpl) ErrorBudget() MetricGenerator {
	return NewConstantMetricGenerator(SLOErrorBudget, 1-normalizeObjective(s.slo.Objective), 9, s.slo.IdLabels)
}

func (s *SLOGeneratorImpl) Period() MetricGenerator {
	fSeconds := time.Duration(s.period).Seconds()
	return NewConstantMetricGenerator(SLOPeriod, fSeconds, 9, s.slo.IdLabels)
}

func (s *SLOGeneratorImpl) Info() MetricGenerator {
	return NewConstantMetricGenerator(SLOInfo, 1, 1, s.slo.IdLabels)
}

// ==== Dependents on SLI
func (s *SLOGeneratorImpl) CurrentBurnRate() MetricGenerator {
	windows := s.Windows()
	return NewBurnRateGenerator(
		SLOCurrentBurnRate,
		s.slo.IdLabels,
		s.SLI(windows[0]),
		s.ErrorBudget(),
		s.options,
	)
}

func (s *SLOGeneratorImpl) PeriodBurnRate() MetricGenerator {
	return NewBurnRateGenerator(
		SLOPeriodBurnRate,
		s.slo.IdLabels,
		s.SLI(s.period),
		s.ErrorBudget(),
		s.options,
	)
}

func (s *SLOGeneratorImpl) ErrorBudgetRemaining() MetricGenerator {
	return NewErrorBudgetRemainingGenerator(
		SLOErrorBudgetRemaining,
		s.slo.IdLabels,
		s.PeriodBurnRate(),
		s.options,
	)
}

func (s *SLOGeneratorImpl) PageAlert() MetricGenerator {
	return NewMWMBAlertGenerator(
		"slo:mwmb_alert:page",
		s.slo.IdLabels,
		s.windows.PageQuick,
		s.windows.PageSlow,
		func(window prommodel.Duration) MetricGenerator {
			return s.SLI(window)
		},
		s.options,
	)
}

func (s *SLOGeneratorImpl) TicketAlert() MetricGenerator {
	return NewMWMBAlertGenerator(
		"slo:mwmb_alert:ticket",
		s.slo.IdLabels,
		s.windows.TicketQuick,
		s.windows.TicketSlow,
		func(window prommodel.Duration) MetricGenerator {
			return s.SLI(window)
		},
		s.options,
	)
}

var _ MWMBSLOGenerator = (*SLOGeneratorImpl)(nil)

type ConstantMetricGenerator struct {
	name      string
	value     float64
	precision int
	idLabels  backend.IdentificationLabels
}

var _ MetricGenerator = (*ConstantMetricGenerator)(nil)

func NewConstantMetricGenerator(id string, value float64, precision int, labels map[string]string) *ConstantMetricGenerator {
	return &ConstantMetricGenerator{
		name:      id,
		value:     value,
		precision: precision,
		idLabels:  labels,
	}
}

func (v *ConstantMetricGenerator) Id() string {
	return v.name
}

func (v *ConstantMetricGenerator) Expr() string {
	precision := fmt.Sprintf("vector(%%.%df)", v.precision)
	return fmt.Sprintf(precision, v.value)
}

func (v *ConstantMetricGenerator) Rule() (rulefmt.RuleNode, error) {
	return rulefmt.RuleNode{
		Record: yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: v.name,
		},
		Expr: yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: v.Expr(),
		},
		Labels: v.idLabels,
	}, nil
}

type SumRateGenerator struct {
	goodExpr string
	window   prommodel.Duration
}

var _ MetricGenerator = (*SumRateGenerator)(nil)

func NewSumRateGenerator(
	goodExpr string,
	window prommodel.Duration,
) *SumRateGenerator {
	return &SumRateGenerator{
		goodExpr: goodExpr,
		window:   window,
	}
}

func (s *SumRateGenerator) Id() string {
	return ""
}

func (s *SumRateGenerator) Expr() string {
	return fmt.Sprintf("sum(rate(%s[%s]))", s.goodExpr, s.window.String())
}

func (s *SumRateGenerator) Rule() (rulefmt.RuleNode, error) {
	return rulefmt.RuleNode{}, fmt.Errorf("partial rules cannot be converted to rulefmt")
}

func NewMWMBSLIGenerator(
	name, goodExpr, totalExpr string,
	window prommodel.Duration,
	idLabels backend.IdentificationLabels,
) *MWMBSLIGenerator {
	goodQuery := NewSumRateGenerator(goodExpr, window)
	totalQuery := NewSumRateGenerator(totalExpr, window)
	return &MWMBSLIGenerator{
		name:       name,
		window:     window,
		goodQuery:  goodQuery,
		totalQuery: totalQuery,
		idLabels:   idLabels,
	}
}

type MWMBSLIGenerator struct {
	name   string
	window prommodel.Duration

	goodQuery  MetricGenerator
	totalQuery MetricGenerator

	idLabels backend.IdentificationLabels
}

var _ MetricGenerator = (*MWMBSLIGenerator)(nil)

func (s *MWMBSLIGenerator) Id() string {
	return s.name + s.window.String()
}

func (s *MWMBSLIGenerator) Expr() string {
	return fmt.Sprintf("1 - ((%s) / (%s))", s.goodQuery.Expr(), s.totalQuery.Expr())
}

func (s *MWMBSLIGenerator) Rule() (rulefmt.RuleNode, error) {
	return rulefmt.RuleNode{
		Record: yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: s.Id(),
		},
		Expr: yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: s.Expr(),
		},
		Labels: s.idLabels,
	}, nil
}

type BurnRateGenerator struct {
	name     string
	idLabels backend.IdentificationLabels

	sli         MetricGenerator
	errorBudget MetricGenerator

	options *SLOGeneratorOptions
}

var _ MetricGenerator = (*BurnRateGenerator)(nil)

func (b *BurnRateGenerator) Id() string {
	return b.name
}

func NewBurnRateGenerator(
	id string,
	idLabels backend.IdentificationLabels,
	sli MetricGenerator,
	errorBudget MetricGenerator,
	options *SLOGeneratorOptions,
) *BurnRateGenerator {
	return &BurnRateGenerator{
		name:        id,
		idLabels:    idLabels,
		sli:         sli,
		errorBudget: errorBudget,
		options:     options,
	}
}

func (b *BurnRateGenerator) Expr() string {
	var numerator, denominator string
	if b.options.optimized {
		numerator = QueryWithLabels(b.sli.Id(), b.idLabels)
		denominator = QueryWithLabels(b.errorBudget.Id(), b.idLabels)
		return fmt.Sprintf(
			"%s / on(%s) group_left %s",
			numerator,
			b.idLabels.JoinOnPrometheus(),
			denominator,
		)
	}
	numerator = b.sli.Expr()
	denominator = b.errorBudget.Expr()
	return fmt.Sprintf(
		"(%s) / (%s)",
		numerator,
		denominator,
	)
}

func (b *BurnRateGenerator) Rule() (rulefmt.RuleNode, error) {
	return rulefmt.RuleNode{
		Record: yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: b.name,
		},
		Expr: yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: b.Expr(),
		},
		Labels: b.idLabels,
	}, nil
}

type ErrorBudgetRemainingGenerator struct {
	name           string
	idLabels       backend.IdentificationLabels
	periodBurnRate MetricGenerator
	options        *SLOGeneratorOptions
}

var _ MetricGenerator = (*ErrorBudgetRemainingGenerator)(nil)

func NewErrorBudgetRemainingGenerator(
	id string,
	idLabels backend.IdentificationLabels,
	periodBurnRate MetricGenerator,
	options *SLOGeneratorOptions,
) *ErrorBudgetRemainingGenerator {
	return &ErrorBudgetRemainingGenerator{
		name:           id,
		idLabels:       idLabels,
		periodBurnRate: periodBurnRate,
		options:        options,
	}
}

func (e *ErrorBudgetRemainingGenerator) Id() string {
	return e.name
}

func (e *ErrorBudgetRemainingGenerator) Expr() string {
	var subExpr string
	if e.options.optimized {
		subExpr = QueryWithLabels(e.periodBurnRate.Id(), e.idLabels)
	} else {
		subExpr = e.periodBurnRate.Expr()
	}
	return fmt.Sprintf("1 - (%s)", subExpr)
}

func (e *ErrorBudgetRemainingGenerator) Rule() (rulefmt.RuleNode, error) {
	return rulefmt.RuleNode{
		Record: yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: e.name,
		},
		Expr: yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: e.Expr(),
		},
		Labels: e.idLabels,
	}, nil
}

var mwmbAlertTplBool = template.Must(template.New("mwmbAlertTpl").Option("missingkey=error").Parse(`
(max({{ .QuickShortMetric }}{{ .MetricFilter}} > bool ({{ .QuickShortBurnFactor }} * {{ .ErrorBudgetRatio }})) 
without ({{ .WindowLabel }}) and 
max({{ .QuickLongMetric }}{{ .MetricFilter}} > bool ({{ .QuickLongBurnFactor }} * {{ .ErrorBudgetRatio }})) without ({{ .WindowLabel }})) 

or 

(max({{ .SlowShortMetric }}{{ .MetricFilter }} > bool ({{ .SlowShortBurnFactor }} * {{ .ErrorBudgetRatio }})) without ({{ .WindowLabel }}) 
and max({{ .SlowQuickMetric }}{{ .MetricFilter }} > bool ({{ .SlowQuickBurnFactor }} * {{ .ErrorBudgetRatio }})) without ({{ .WindowLabel }}))`))

// TODO : add a description of how these alerts are structured
type MWMBAlertGenerator struct {
	name        string
	quickWindow Window
	slowWindow  Window
	idLabels    backend.IdentificationLabels
	options     *SLOGeneratorOptions
	errorRate   func(model.Duration) MetricGenerator
}

var _ MetricGenerator = (*MWMBAlertGenerator)(nil)

func NewMWMBAlertGenerator(
	name string,
	idLabels backend.IdentificationLabels,
	quickWindow, slowWindow Window,
	errorRate func(model.Duration) MetricGenerator,
	options *SLOGeneratorOptions,
) *MWMBAlertGenerator {
	return &MWMBAlertGenerator{
		name:        name,
		idLabels:    idLabels,
		quickWindow: quickWindow,
		slowWindow:  slowWindow,
		errorRate:   errorRate,
		options:     options,
	}
}

func (m *MWMBAlertGenerator) Id() string {
	return m.name
}

func (m *MWMBAlertGenerator) Expr() string {
	construct := func(dur prommodel.Duration) string {
		return m.errorRate(dur).Expr()
	}
	if m.options.optimized {
		construct = func(dur prommodel.Duration) string {
			return fmt.Sprintf("%s{%s}", m.errorRate(dur).Id(), m.idLabels.ToLabels().ConstructPrometheus())
		}

	}
	shortWindowQuickExpr := fmt.Sprintf("(%s) > (%.9f * %.9f)", construct(m.quickWindow.ShortWindow), m.quickWindow.GetShortBurnRateFactor(), m.quickWindow.ErrorBudgetPercent)
	shortWindowLongExpr := fmt.Sprintf("(%s) > (%.9f * %.9f)", construct(m.quickWindow.LongWindow), m.quickWindow.GetLongBurnRateFactor(), m.quickWindow.ErrorBudgetPercent)
	longWindowQuickExpr := fmt.Sprintf("(%s) > (%.9f * %.9f)", construct(m.slowWindow.ShortWindow), m.slowWindow.GetShortBurnRateFactor(), m.slowWindow.ErrorBudgetPercent)
	longWindowLongExpr := fmt.Sprintf("(%s) > (%.9f * %.9f)", construct(m.slowWindow.LongWindow), m.slowWindow.GetLongBurnRateFactor(), m.slowWindow.ErrorBudgetPercent)

	return fmt.Sprintf(
		"(max((%s)) and max((%s))) or (max(%s) and max(%s))",
		shortWindowQuickExpr,
		shortWindowLongExpr,
		longWindowQuickExpr,
		longWindowLongExpr,
	)
}

func (m *MWMBAlertGenerator) Rule() (rulefmt.RuleNode, error) {
	return rulefmt.RuleNode{
		Alert: yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: m.Id(),
		},
		Expr: yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: m.Expr(),
		},
		Labels: m.idLabels,
	}, nil
}
