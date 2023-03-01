package templates_test

import (
	"bytes"
	"reflect"
	"time"

	"cuelang.org/go/pkg/strings"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/drivers/config"
	"github.com/rancher/opni/pkg/alerting/interfaces"
	"github.com/rancher/opni/pkg/alerting/templates"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"

	"google.golang.org/protobuf/types/known/durationpb"

	"text/template"

	amtemplate "github.com/prometheus/alertmanager/template"
)

func init() {
	templates.RegisterNewAlertManagerDefaults(amtemplate.DefaultFuncs, templates.DefaultTemplateFuncs)
}

type TestMessage interface {
	interfaces.Routable
	Validate() error
}

var _ = DescribeTable("Message templating",
	func(incomingMsg TestMessage, status string, headerContains, bodyContains []string) {
		Expect(incomingMsg).ToNot(BeNil())
		Expect(incomingMsg.Validate()).ToNot(HaveOccurred())

		msg := &config.WebhookMessage{
			Receiver: "test",
			Status:   "firing",
			Alerts: config.Alerts{
				{
					Status:      status,
					Labels:      incomingMsg.GetRoutingLabels(),
					Annotations: incomingMsg.GetRoutingAnnotations(),
					StartsAt:    time.Now(),
				},
			},
			Version:         "v4",
			ExternalURL:     "http://localhost:9093",
			TruncatedAlerts: 0,
		}

		headerTmpl, err := template.New("").Funcs(
			template.FuncMap(amtemplate.DefaultFuncs),
		).Parse(templates.HeaderTemplate())
		Expect(err).ToNot(HaveOccurred())

		bodyTmpl, err := template.New("").Funcs(
			template.FuncMap(amtemplate.DefaultFuncs),
		).Parse(templates.BodyTemplate())
		Expect(err).ToNot(HaveOccurred())

		var (
			b1 bytes.Buffer
			b2 bytes.Buffer
		)
		err = headerTmpl.Execute(&b1, msg)
		Expect(err).ToNot(HaveOccurred())

		err = bodyTmpl.Execute(&b2, msg)
		Expect(err).ToNot(HaveOccurred())

		s1, s2 := b1.String(), b2.String()

		for _, s := range headerContains {
			Expect(s1).To(ContainSubstring(s))
		}

		for _, s := range bodyContains {
			Expect(s2).To(ContainSubstring(s))
		}
		By("verifying that we output pretty timestamps in messages")
		tsFunc, ok := amtemplate.DefaultFuncs["formatTime"]
		Expect(ok).To(BeTrue())

		v := reflect.ValueOf(tsFunc)
		Expect(v.Kind()).To(Equal(reflect.Func))

		prettyTs := v.Call([]reflect.Value{reflect.ValueOf(msg.Alerts[0].StartsAt)})
		Expect(prettyTs).To(HaveLen(2))

		Expect(prettyTs[1].IsNil()).To(BeTrue())
		Expect(prettyTs[0].Kind()).To(Equal(reflect.String))
		Expect(prettyTs[0].String()).ToNot(HaveLen(0))
		Expect(strings.Contains(s1, prettyTs[0].String()) || strings.Contains(s2, prettyTs[0].String())).To(BeTrue())
	},
	Entry(
		"firing alarm uses user's custom title and body",
		&alertingv1.AlertCondition{
			Name:        "condition 1",
			Description: "condition 1 description",
			Severity:    alertingv1.OpniSeverity_Info,
			AlertType: &alertingv1.AlertTypeDetails{
				Type: &alertingv1.AlertTypeDetails_System{
					System: &alertingv1.AlertConditionSystem{
						ClusterId: &corev1.Reference{Id: uuid.New().String()},
						Timeout:   durationpb.New(10 * time.Minute),
					},
				},
			},
			AttachedEndpoints: &alertingv1.AttachedEndpoints{
				Items: []*alertingv1.AttachedEndpoint{
					{EndpointId: uuid.New().String()},
				},
				Details: &alertingv1.EndpointImplementation{
					Title: "hello",
					Body:  "world",
				},
			},
		},
		"firing",
		[]string{alertingv1.OpniSeverity_Info.String(), "Alarm", "condition 1"},
		[]string{"hello", "world"},
	),
	Entry(
		"firing alarm falls back to name & description",
		&alertingv1.AlertCondition{
			Name:        "condition 1",
			Description: "condition 1 description",
			Severity:    alertingv1.OpniSeverity_Warning,
			AlertType: &alertingv1.AlertTypeDetails{
				Type: &alertingv1.AlertTypeDetails_System{
					System: &alertingv1.AlertConditionSystem{
						ClusterId: &corev1.Reference{Id: uuid.New().String()},
						Timeout:   durationpb.New(10 * time.Minute),
					},
				},
			},
		},
		"firing",
		[]string{"condition 1", "Warning", alertingv1.OpniSeverity_Warning.String(), "firing"},
		[]string{"condition 1", "condition 1 description"},
	),
	Entry(
		"opni alarm is resolved",
		&alertingv1.AlertCondition{
			Name:        "test header 2",
			Description: "body 2",
			Severity:    alertingv1.OpniSeverity_Error,
			AlertType: &alertingv1.AlertTypeDetails{
				Type: &alertingv1.AlertTypeDetails_System{
					System: &alertingv1.AlertConditionSystem{
						ClusterId: &corev1.Reference{Id: uuid.New().String()},
						Timeout:   durationpb.New(10 * time.Minute),
					},
				},
			},
		},
		"resolved",
		[]string{"test header 2", alertingv1.OpniSeverity_Error.String(), "Alarm", "resolved"},
		[]string{"test header 2", "body 2"},
	),
	Entry(
		"critical notification",
		&alertingv1.Notification{
			Title: "notification title",
			Body:  "notification body",
			Properties: map[string]string{
				alertingv1.NotificationPropertySeverity: "Critical",
			},
		},
		"firing",
		[]string{"Critical", "Notification", "notification title"},
		[]string{"notification body"},
	),
)
