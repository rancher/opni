package messaging

import (
	"context"
	"sync"

	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
)

type MessagingNode struct {
	// conditionId -> subsriber pull context cancel func
	systemConditionUpdateListeners map[string]context.CancelFunc
	systemConditionMu              sync.Mutex

	cortexClusterStatusListeners map[string]context.CancelFunc
	cortexClusterStatusChannels  map[string]chan *cortexadmin.CortexStatus
	cortexStatusConditionMu      sync.RWMutex
}

func NewMessagingNode() *MessagingNode {
	return &MessagingNode{
		systemConditionUpdateListeners: make(map[string]context.CancelFunc),
		cortexClusterStatusListeners:   make(map[string]context.CancelFunc),
		cortexClusterStatusChannels:    make(map[string]chan *cortexadmin.CortexStatus),
	}
}

func (n *MessagingNode) AddCortexClusterStatusListener(conditionId string, ca context.CancelFunc) {
	n.cortexStatusConditionMu.Lock()
	defer n.cortexStatusConditionMu.Unlock()
	if _, ok := n.cortexClusterStatusListeners[conditionId]; ok {
		//existing goroutine, cancel it
		n.cortexClusterStatusListeners[conditionId]()
	}
	n.cortexClusterStatusListeners[conditionId] = ca
	n.cortexClusterStatusChannels[conditionId] = make(chan *cortexadmin.CortexStatus, 64)
}

func (n *MessagingNode) RemoveCortexClusterStatusListener(conditionId string) {
	n.cortexStatusConditionMu.Lock()
	defer n.cortexStatusConditionMu.Unlock()
	if ca, ok := n.cortexClusterStatusListeners[conditionId]; ok {
		ca()
	}
	delete(n.cortexClusterStatusListeners, conditionId)
	delete(n.cortexClusterStatusChannels, conditionId)
}

func (n *MessagingNode) GetWatcher(conditionId string) chan *cortexadmin.CortexStatus {
	n.cortexStatusConditionMu.RLock()
	defer n.cortexStatusConditionMu.RUnlock()
	return n.cortexClusterStatusChannels[conditionId]
}

func (n *MessagingNode) ListCortexStatusListeners() []chan *cortexadmin.CortexStatus {
	n.cortexStatusConditionMu.Lock()
	defer n.cortexStatusConditionMu.Unlock()
	res := []chan *cortexadmin.CortexStatus{}
	for _, k := range n.cortexClusterStatusChannels {
		res = append(res, k)
	}
	return res
}

func (n *MessagingNode) AddSystemConfigListener(conditionId string, ca context.CancelFunc) {
	n.conditionMu.Lock()
	defer n.conditionMu.Unlock()
	if oldCa, ok := n.systemConditionUpdateListeners[conditionId]; ok {
		//existing goroutine, cancel it
		oldCa()
	}
	n.systemConditionUpdateListeners[conditionId] = ca
}

func (n *MessagingNode) RemoveConfigListener(conditionId string) {
	n.systemConditionMu.Lock()
	defer n.systemConditionMu.Unlock()
	if ca, ok := n.systemConditionUpdateListeners[conditionId]; ok {
		ca()
	}
	delete(n.systemConditionUpdateListeners, conditionId)
}
