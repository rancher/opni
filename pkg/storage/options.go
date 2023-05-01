package storage

import corev1 "github.com/rancher/opni/pkg/apis/core/v1"

type TokenCreateOptions struct {
	Labels       map[string]string
	Capabilities []*corev1.TokenCapability
	OneTime      bool
}

func NewTokenCreateOptions() TokenCreateOptions {
	return TokenCreateOptions{
		Labels:       map[string]string{},
		Capabilities: []*corev1.TokenCapability{},
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

func WithCapabilities(capabilities []*corev1.TokenCapability) TokenCreateOption {
	return func(o *TokenCreateOptions) {
		o.Capabilities = capabilities
	}
}

func WithOneTime() TokenCreateOption {
	return func(o *TokenCreateOptions) {
		o.OneTime = true
	}
}

type AlertFilterOptions struct {
	Labels map[string]string
	Range  *corev1.TimeRange
}
