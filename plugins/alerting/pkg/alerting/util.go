package alerting

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/rancher/opni/pkg/alerting/drivers/backend"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/samber/lo"
)

// as opposed to an errGroup which stops on the first error,
// this is a best effort to run all tasks and return all errors
type independentErrGroup struct {
	errMu sync.Mutex
	errs  []error
	sync.WaitGroup
}

func (i *independentErrGroup) Add(tasks int) {
	i.WaitGroup.Add(tasks)
}

func (i *independentErrGroup) Done() {
	i.WaitGroup.Done()
}

func (i *independentErrGroup) Wait() {
	i.WaitGroup.Wait()
}

func (i *independentErrGroup) AddError(err error) {
	i.errMu.Lock()
	defer i.errMu.Unlock()
	i.errs = append(i.errs, err)
}

func (i *independentErrGroup) Error() error {
	if len(i.errs) == 0 {
		return nil
	}
	duped := map[string]struct{}{}
	resErr := []string{}
	for _, err := range i.errs {
		if _, ok := duped[err.Error()]; !ok {
			duped[err.Error()] = struct{}{}
			resErr = append(resErr, err.Error())
		}
	}
	sort.Strings(resErr)
	return fmt.Errorf(strings.Join(resErr, ","))
}

func statusFromLoadedReceivers(
	cond *alertingv1.AlertCondition,
	matchers []*labels.Matcher,
	requiredReceivers,
	loadedReceivers []string,
) *alertingv1.AlertStatusResponse {
	if len(requiredReceivers) == 0 ||
		cond.AttachedEndpoints == nil ||
		len(cond.AttachedEndpoints.Items) == 0 {
		return &alertingv1.AlertStatusResponse{
			State: alertingv1.AlertConditionState_Ok,
		}
	}
	matchingReceivers := lo.Intersect(requiredReceivers, loadedReceivers)
	if len(matchers) != 0 && len(matchingReceivers) != len(requiredReceivers) {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Pending,
			Reason: "alarm dependencies are updating",
		}
	}
	return &alertingv1.AlertStatusResponse{
		State: alertingv1.AlertConditionState_Ok,
	}
}

func statusFromAlertGroup(
	matchers []*labels.Matcher,
	alertGroup []backend.GettableAlert,
) *alertingv1.AlertStatusResponse {
	if len(matchers) == 0 {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Pending,
			Reason: "alarm dependencies are updating",
		}
	}
	for _, alert := range alertGroup {
		// must match all matchers from the router spec to the alert's labels
		if !lo.EveryBy(matchers, func(m *labels.Matcher) bool {
			for labelName, label := range alert.Labels {
				if m.Name == labelName && m.Matches(label) {
					return true
				}
			}
			return false
		}) {
			continue // these are not the alerts you are looking for
		}
		switch *alert.Status.State {
		case models.AlertStatusStateSuppressed:
			return &alertingv1.AlertStatusResponse{
				State: alertingv1.AlertConditionState_Silenced,
			}
		case models.AlertStatusStateActive:
			return &alertingv1.AlertStatusResponse{
				State: alertingv1.AlertConditionState_Firing,
			}
		case models.AlertStatusStateUnprocessed:
			// in our case unprocessed means it has arrived for firing
			return &alertingv1.AlertStatusResponse{
				State: alertingv1.AlertConditionState_Firing,
			}
		default:
			return &alertingv1.AlertStatusResponse{
				State: alertingv1.AlertConditionState_Ok,
			}
		}
	}
	return &alertingv1.AlertStatusResponse{
		State: alertingv1.AlertConditionState_Ok,
	}
}
