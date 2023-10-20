package events

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"log/slog"

	"github.com/opensearch-project/opensearch-go/opensearchutil"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/logging/apis/node"
	"github.com/rancher/opni/plugins/logging/pkg/agent/drivers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	clientset kubernetes.Interface
	queue     workqueue.RateLimitingInterface
	informer  informercorev1.EventInformer
	logger    *slog.Logger
	state     scrapeState
	namespace string
}

type scrapeState struct {
	sync.Mutex
	running bool
	stopCh  chan struct{}
}

type EventCollectorOptions struct {
	maxRetries int
	restConfig *rest.Config
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

func WithRestConfig(restConfig *rest.Config) EventCollectorOption {
	return func(o *EventCollectorOptions) {
		o.restConfig = restConfig
	}
}

func NewEventCollector(
	logger *slog.Logger,
	opts ...EventCollectorOption,
) (*EventCollector, error) {
	options := EventCollectorOptions{
		maxRetries: 3,
	}
	options.apply(opts...)

	namespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		return nil, fmt.Errorf("POD_NAMESPACE environment variable not set")
	}

	if options.restConfig == nil {
		restConfig, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create rest config: %w", err)
		}
		options.restConfig = restConfig
	}

	clientset, err := kubernetes.NewForConfig(options.restConfig)
	if err != nil {
		return nil, err
	}

	factory := informers.NewSharedInformerFactory(clientset, time.Minute)
	informer := factory.Core().V1().Events()

	return &EventCollector{
		EventCollectorOptions: options,
		clientset:             clientset,
		informer:              informer,
		logger:                logger,
		namespace:             namespace,
	}, nil
}

var _ drivers.LoggingNodeDriver = (*EventCollector)(nil)

func (c *EventCollector) Name() string {
	return "event-collector"
}

func (c *EventCollector) ConfigureNode(config *node.LoggingCapabilityConfig) {
	c.state.Lock()
	defer c.state.Unlock()
	if config.GetEnabled() {
		if c.state.running {
			return
		}
		c.state.stopCh = make(chan struct{})
		c.queue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
		go func() {
			err := c.run(c.state.stopCh)
			if err != nil {
				c.logger.Error("failed to start events", logger.Err(err))
				c.state.Lock()
				close(c.state.stopCh)
				c.state.running = false
				c.state.Unlock()
			}
		}()
		c.state.running = true
		return
	}

	if c.state.running {
		close(c.state.stopCh)
		c.queue.ShutDown()
		c.state.running = false
	}

}

func (c *EventCollector) run(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
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
		c.logger.Info("queue shutdown, halting event shipping")
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
	systemNamespace, err := c.clientset.CoreV1().Namespaces().Get(context.TODO(), "kube-system", metav1.GetOptions{})
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

	endpoint := fmt.Sprintf("http://opni-shipper.%s:2021/log/ingest", c.namespace)

	req, err := http.NewRequest(http.MethodPost, endpoint, opensearchutil.NewJSONReader(output))
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
