package storage

import "github.com/rancher/opni-monitoring/pkg/core"

type TokenCreateOptions struct {
	Labels       map[string]string
	Capabilities []*core.TokenCapability
}

func NewTokenCreateOptions() TokenCreateOptions {
	return TokenCreateOptions{
		Labels:       map[string]string{},
		Capabilities: []*core.TokenCapability{},
	}
}

type TokenCreateOption func(*TokenCreateOptions)

func (o *TokenCreateOptions) Apply(opts ...TokenCreateOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithLabels(labels map[string]string) TokenCreateOption {
	return func(o *TokenCreateOptions) {
		o.Labels = labels
	}
}

func WithCapabilities(capabilities []*core.TokenCapability) TokenCreateOption {
	return func(o *TokenCreateOptions) {
		o.Capabilities = capabilities
	}
}
