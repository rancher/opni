package messaging

type MessagingNode struct {
	listeners map[string]chan<- []byte
}

func NewMessagingNode() *MessagingNode {
	return &MessagingNode{
		listeners: make(map[string]chan<- []byte),
	}
}

func (n *MessagingNode) AddMessagingListener(conditionId string, ch chan<- []byte) {
	n.listeners[conditionId] = ch
}
