package messaging

import (
	"context"
	"sync"
)

type MessagingNode struct {
	// conditionId -> subsriber pull context cancel func
	systemConditionUpdateListeners map[string]context.CancelFunc

	conditionMu sync.Mutex
}

func NewMessagingNode() *MessagingNode {
	return &MessagingNode{
		systemConditionUpdateListeners: make(map[string]context.CancelFunc),
	}
}

func (n *MessagingNode) AddSystemConfigListener(conditionId string, ca context.CancelFunc) {
	n.conditionMu.Lock()
	defer n.conditionMu.Unlock()
	if _, ok := n.systemConditionUpdateListeners[conditionId]; ok {
		//existing goroutine, cancel it
		ca()
	}
	n.systemConditionUpdateListeners[conditionId] = ca
}

func (n *MessagingNode) RemoveConfigListener(conditionId string) {
	n.conditionMu.Lock()
	defer n.conditionMu.Unlock()
	if ca, ok := n.systemConditionUpdateListeners[conditionId]; ok {
		ca()
	}
	delete(n.systemConditionUpdateListeners, conditionId)

}
