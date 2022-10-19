package messaging

import (
	"context"
	"sync"

	"github.com/nats-io/nats.go"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/util/future"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
)

type MessagingNode struct {
	systemConditionUpdateListeners map[*corev1.Reference]chan<- *alertingv1alpha.AlertCondition

	conditionMu sync.Mutex
}

func NewMessagingNode(newNatsConn future.Future[*nats.Conn]) *MessagingNode {
	return &MessagingNode{
		systemConditionUpdateListeners: make(map[*corev1.Reference]chan<- *alertingv1alpha.AlertCondition),
	}
}

func (n *MessagingNode) AddSystemConfigListener(condition *corev1.Reference, ch chan<- *alertingv1alpha.AlertCondition) {
	n.systemConditionUpdateListeners[condition] = ch
}

func (n *MessagingNode) RemoveConfigListener(condition *corev1.Reference) {
	delete(n.systemConditionUpdateListeners, condition)
}

func NewConfigListenerFunc(ctx context.Context, fn func(*alertingv1alpha.AlertCondition)) chan<- *alertingv1alpha.AlertCondition {
	ch := make(chan *alertingv1alpha.AlertCondition)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case c := <-ch:
				fn(c)
			}
		}
	}()
	return ch
}
