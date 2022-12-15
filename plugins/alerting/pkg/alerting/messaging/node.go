package messaging

import (
	"context"
	"sync"
	"sync/atomic"
)

type EvaluatorContext struct {
	Ctx     context.Context
	Cancel  context.CancelFunc
	running atomic.Bool
}

type MessagingNode struct {
	// conditionId -> subsriber pull context cancel func
	systemConditionUpdateListeners map[string]EvaluatorContext
	systemConditionMu              sync.Mutex
}

func NewMessagingNode() *MessagingNode {
	return &MessagingNode{
		systemConditionUpdateListeners: make(map[string]EvaluatorContext),
	}
}
func (n *MessagingNode) AddSystemConfigListener(conditionId string, eCtx EvaluatorContext) {
	n.systemConditionMu.Lock()
	defer n.systemConditionMu.Unlock()
	if oldContext, ok := n.systemConditionUpdateListeners[conditionId]; ok {
		//existing goroutine, cancel it
		oldContext.Cancel()
	}
	eCtx.running.Store(true)
	n.systemConditionUpdateListeners[conditionId] = eCtx
	go func() {
		defer eCtx.running.Store(false)
		<-eCtx.Ctx.Done()
	}()
}

func (n *MessagingNode) RemoveConfigListener(conditionId string) {
	n.systemConditionMu.Lock()
	defer n.systemConditionMu.Unlock()
	if oldContext, ok := n.systemConditionUpdateListeners[conditionId]; ok {
		oldContext.Cancel()
	}
	delete(n.systemConditionUpdateListeners, conditionId)
}

func (n *MessagingNode) IsRunning(conditionId string) bool {
	n.systemConditionMu.Lock()
	defer n.systemConditionMu.Unlock()
	eCtx, ok := n.systemConditionUpdateListeners[conditionId]
	return ok && eCtx.running.Load()
}
