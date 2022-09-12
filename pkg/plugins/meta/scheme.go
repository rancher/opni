package meta

import (
	"github.com/hashicorp/go-plugin"
)

type Scheme interface {
	Add(string, plugin.Plugin)
	PluginMap() map[string]plugin.Plugin
	Mode() PluginMode
}

type scheme struct {
	SchemeOptions
	plugins map[string]plugin.Plugin
}

func (s *scheme) PluginMap() map[string]plugin.Plugin {
	return s.plugins
}

func (s *scheme) Add(id string, impl plugin.Plugin) {
	if _, ok := s.plugins[id]; ok {
		panic("plugin already added")
	}
	s.plugins[id] = impl
}

func (s *scheme) Mode() PluginMode {
	return s.mode
}

type SchemeOptions struct {
	mode PluginMode
}

type SchemeOption func(*SchemeOptions)

func (o *SchemeOptions) apply(opts ...SchemeOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithMode(mode PluginMode) SchemeOption {
	return func(o *SchemeOptions) {
		o.mode = mode
	}
}

func NewScheme(opts ...SchemeOption) Scheme {
	options := SchemeOptions{}
	options.apply(opts...)
	return &scheme{
		SchemeOptions: options,
		plugins:       map[string]plugin.Plugin{},
	}
}
