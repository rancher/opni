package events

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"log/slog"

	"github.com/opensearch-project/opensearch-go/opensearchutil"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	informercorev1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type EventOutput struct {
	*corev1.Event
	ClusterID string    `json:"cluster_id,omitempty"`
	LogType   string    `json:"log_type,omitempty"`
	PodName   string    `json:"pod_name,omitempty"`
	Time      time.Time `json:"time,omitempty"`
}

type timestampedEvent struct {
	key  string
	time time.Time
}

type EventCollector struct {
	EventCollectorOptions
	ctx       context.Context
	clientset kubernetes.Interface
	queue     workqueue.RateLimitingInterface
	informer  informercorev1.EventInformer
	endpoint  string
	logger    *slog.Logger
}

type EventCollectorOptions struct {
	maxRetries int
}

type EventCollectorOption func(*EventCollectorOptions)

func (o *EventCollectorOptions) apply(opts ...EventCollectorOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithMaxRetries(retries int) EventCollectorOption {
	return func(o *EventCollectorOptions) {
		o.maxRetries = retries
	}
}

func NewEventCollector(ctx context.Context, endpoint string, opts ...EventCollectorOption) *EventCollector {
	options := EventCollectorOptions{
		maxRetries: 3,
	}
	options.apply(opts...)

	lg := logger.New().WithGroup("events")
	config, err := rest.InClusterConfig()
	if err != nil {
		lg.Error(fmt.Sprintf("failed to create config: %s", err))
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		lg.Error(fmt.Sprintf("failed to create clientset: %s", err))
		os.Exit(1)
	}

	factory := informers.NewSharedInformerFactory(clientset, time.Minute)
	informer := factory.Core().V1().Events()

	return &EventCollector{
		EventCollectorOptions: options,
		ctx:                   ctx,
		clientset:             clientset,
		queue:                 workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		informer:              informer,
		endpoint:              endpoint,
		logger:                lg,
	}
}

func (c *EventCollector) Run(stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.queue.ShutDown()

	c.informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			c.enqueueEvent(obj)
		},
		UpdateFunc: func(old, new interface{}) {
			newObj := new.(*corev1.Event)
			oldObj := old.(*corev1.Event)
			if newObj.ResourceVersion == oldObj.ResourceVersion {
				return
			}
			c.enqueueEvent(newObj)
		},
		DeleteFunc: func(obj interface{}) {
			c.enqueueEvent(obj)
		},
	})

	c.logger.Info("starting event collector")
	go c.informer.Informer().Run(stopCh)

	if ok := cache.WaitForCacheSync(stopCh, c.informer.Informer().HasSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	c.logger.Info("collector started")
	wait.Until(c.runWorker, time.Second, stopCh)

	c.logger.Info("shutting down collector")
	return nil
}

func (c *EventCollector) runWorker() {
	for {
		if !c.processNextItem() {
			return
		}
	}
}

func (c *EventCollector) enqueueEvent(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		c.logger.Error(fmt.Sprintf("could't get key for event %+v: %s", obj, err))
	}
	c.queue.Add(timestampedEvent{
		key:  key,
		time: time.Now(),
	})
}

func (c *EventCollector) processNextItem() bool {
	event, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(event)

	err := c.processItem(event)
	if err == nil {
		c.queue.Forget(event)
		return true
	}
	if c.maxRetries == 0 || c.queue.NumRequeues(event) < c.maxRetries {
		c.logger.Warn(fmt.Sprintf("failed to process event %s, requeueing: %v", event, err))
		c.queue.AddRateLimited(event)
		return true
	}

	c.logger.Error(fmt.Sprintf("failed to process event %s, giving up: %v", event, err))
	c.queue.Forget(event)
	utilruntime.HandleError(err)
	return true
}

func (c *EventCollector) processItem(obj interface{}) error {
	eventObj := obj.(timestampedEvent)
	event, _, err := c.informer.Informer().GetIndexer().GetByKey(eventObj.key)
	if err != nil {
		return err
	}

	if event == nil || util.IsInterfaceNil(event) {
		c.logger.Info("nil event, skipping")
		return nil
	}

	return c.shipEvent(event.(*corev1.Event), eventObj.time)
}

func (c *EventCollector) shipEvent(event *corev1.Event, timestamp time.Time) error {
	systemNamespace, err := c.clientset.CoreV1().Namespaces().Get(c.ctx, "kube-system", metav1.GetOptions{})
	if err != nil {
		return err
	}

	output := []EventOutput{
		{
			Event:     event,
			ClusterID: string(systemNamespace.GetUID()),
			LogType:   "event",
			PodName: func() string {
				if event.InvolvedObject.Kind == "Pod" {
					return event.InvolvedObject.Name
				}
				return ""
			}(),
			Time: timestamp,
		},
	}

	req, err := http.NewRequest(http.MethodPost, c.endpoint, opensearchutil.NewJSONReader(output))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	client := http.DefaultClient
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("failed to post to shipper: %s", resp.Status)
	}

	return nil
}
