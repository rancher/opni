package conditions_test

import (
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"

	. "github.com/onsi/ginkgo/v2"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"

	"github.com/rancher/opni/pkg/test"
)

var testConditionImplementationReference *alertingv1alpha.AlertConditionWithId
var slackId *corev1.Reference
var emailId *corev1.Reference

var _ = Describe("Alerting Conditions integration tests", Ordered, Label(test.Unit, test.Slow), func() {

})
