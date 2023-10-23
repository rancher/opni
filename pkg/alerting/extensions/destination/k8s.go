package destination

import (
	"context"
	"fmt"
	"os"

	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/pkg/alerting/drivers/config"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"log/slog"

	typedv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"

	"k8s.io/client-go/tools/record"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	apis.InitScheme(scheme)
}

type K8sDestination struct {
	lg *slog.Logger

	namespace string
	recorder  record.EventRecorder
}

type EventSinkImpl struct {
	Interface typedv1.EventInterface
}

// Create takes the representation of a event and creates it. Returns the server's representation of the event, and an error, if there is any.
func (e *EventSinkImpl) Create(event *corev1.Event) (*corev1.Event, error) {
	if event.Namespace == "" {
		return nil, fmt.Errorf("can't create an event with empty namespace")
	}
	return e.Interface.Create(context.TODO(), event, metav1.CreateOptions{})
}

// Update takes the representation of a event and updates it. Returns the server's representation of the event, and an error, if there is any.
func (e *EventSinkImpl) Update(event *corev1.Event) (*corev1.Event, error) {
	if event.Namespace == "" {
		return nil, fmt.Errorf("can't update an event with empty namespace")
	}
	return e.Interface.Update(context.TODO(), event, metav1.UpdateOptions{})
}

// Patch applies the patch and returns the patched event, and an error, if there is any.
func (e *EventSinkImpl) Patch(event *corev1.Event, data []byte) (*corev1.Event, error) {
	if event.Namespace == "" {
		return nil, fmt.Errorf("can't patch an event with empty namespace")
	}
	return e.Interface.Patch(context.TODO(), event.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{})
}

func NewEventSink(
	eventClient typedv1.EventInterface,
) record.EventSink {
	return &EventSinkImpl{
		Interface: eventClient,
	}
}

func NewK8sDestination(
	lg *slog.Logger,
) (Destination, error) {
	ns, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		return nil, fmt.Errorf("failed to find namespace to send kubernetes events to")
	}
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	clientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	eventsClient := clientSet.CoreV1().Events(ns)
	broadcaster := record.NewBroadcaster()
	k := &K8sDestination{
		recorder:  broadcaster.NewRecorder(scheme, corev1.EventSource{Component: "opni-alerting"}),
		namespace: ns,
	}
	defer broadcaster.StartRecordingToSink(
		NewEventSink(eventsClient),
	)
	k.lg = lg.With("destination", k.Name())
	return k, nil
}

func (k K8sDestination) Name() string {
	return "k8s"
}

func (k K8sDestination) pushEvents(alerts []config.Alert) {
	for _, alert := range alerts {
		alert := alert
		header, ok := alert.GetHeader()
		if !ok {
			header = missingTitle
		}
		summary, ok := alert.GetSummary()
		if !ok {
			summary = missingBody
		}
		k.recorder.AnnotatedEventf(&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      shared.AlertmanagerService,
				Namespace: k.namespace,
			},
		}, lo.Assign(alert.Annotations, alert.Labels), corev1.EventTypeWarning, header, summary)
	}
}

func (k K8sDestination) Push(_ context.Context, msg config.WebhookMessage) error {
	k.pushEvents(msg.Alerts)
	return nil
}
