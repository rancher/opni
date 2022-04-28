package meta

import (
	"github.com/hashicorp/go-plugin"
)

type Scheme interface {
	Add(string, plugin.Plugin)
	PluginMap() map[string]plugin.Plugin
}

type scheme struct {
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

func NewScheme() Scheme {
	return &scheme{
		plugins: map[string]plugin.Plugin{},
	}
}
