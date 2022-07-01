package modules

import (
	"emperror.dev/errors"
	"github.com/prometheus/client_golang/prometheus"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/samber/lo"
	"go.uber.org/zap"
)

type ModuleContext struct {
	Log    *zap.SugaredLogger
	Client managementv1.ManagementClient
	Reg    prometheus.Registerer
}

type Module interface {
	Run(ctx waitctx.PermissiveContext, mc *ModuleContext) error
}

type NamedModule interface {
	Module
	Name() string
}

type namedModule struct {
	Module
	name string
}

func (m *namedModule) Name() string {
	return m.name
}

type ModuleSet struct {
	modules map[string]NamedModule
}

func NewModuleSet() *ModuleSet {
	return &ModuleSet{
		modules: make(map[string]NamedModule),
	}
}

func (s *ModuleSet) Add(name string, module Module) {
	if _, ok := s.modules[name]; ok {
		panic(errors.Errorf("RT module %s already registered", name))
	}
	s.modules[name] = &namedModule{
		Module: module,
		name:   name,
	}
}

func (s *ModuleSet) Modules() []NamedModule {
	return lo.Values(s.modules)
}
