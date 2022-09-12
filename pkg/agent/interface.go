package agent

import "context"

type AgentInterface interface {
	ListenAndServe(ctx context.Context) error
	ListenAddress() string
}
