package plugins

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type LoaderInterface interface {
	// Adds a hook to the loader, which will be invoked at a specific time according
	// to the hook type. See the specific hooks for more information.
	Hook(h any)
}

type noopLoader struct{}

func (noopLoader) Hook(h any) {
	if c, ok := h.(hooks.LoadingCompletedHook); ok {
		go c.Invoke(0)
	}
}

var NoopLoader = noopLoader{}

type activePlugin struct {
	md     meta.PluginMeta
	client *plugin.GRPCClient
	raw    any
}

type hook[T any] struct {
	hook   T
	caller string
}

type PluginLoader struct {
	logger *zap.SugaredLogger

	hooksMu        sync.RWMutex
	pluginsMu      sync.RWMutex
	loadHooks      []hook[hooks.PluginLoadHook]
	completedHooks []hook[hooks.LoadingCompletedHook]
	activePlugins  []activePlugin
	completed      *atomic.Bool
}

func NewPluginLoader() *PluginLoader {
	return &PluginLoader{
		logger:    logger.New().Named("pluginloader"),
		completed: atomic.NewBool(false),
	}
}

func (p *PluginLoader) Hook(h any) {
	if p.completed.Load() {
		p.hooksMu.RLock()
		defer p.hooksMu.RUnlock()
	} else {
		p.hooksMu.Lock()
		defer p.hooksMu.Unlock()
	}
	// save the caller so we can log it later if needed
	_, file, line, _ := runtime.Caller(1)
	caller := fmt.Sprintf("%s:%d", file, line)

	switch h := h.(type) {
	case hooks.PluginLoadHook:
		p.loadHooks = append(p.loadHooks, hook[hooks.PluginLoadHook]{
			hook:   h,
			caller: caller,
		})

		// late join
		p.pluginsMu.RLock()
		defer p.pluginsMu.RUnlock()
		for _, ap := range p.activePlugins {
			if h.ShouldInvoke(ap.raw) {
				h.Invoke(ap.raw, ap.md, ap.client.Conn)
			}
		}
	case hooks.LoadingCompletedHook:
		p.completedHooks = append(p.completedHooks, hook[hooks.LoadingCompletedHook]{
			hook:   h,
			caller: caller,
		})

		// late join
		if p.completed.Load() {
			go h.Invoke(len(p.activePlugins))
		}
	}
}

// LoadOne loads a single plugin. It invokes PluginLoadHooks for the type of
// the plugin being loaded and will block until all load hooks have completed.
func (p *PluginLoader) LoadOne(ctx context.Context, md meta.PluginMeta, cc *plugin.ClientConfig) {
	p.ensureNotCompleted()
	tracer := otel.Tracer("pluginloader")
	tc, span := tracer.Start(ctx, "LoadOne",
		trace.WithAttributes(attribute.String("plugin", md.Module)))
	defer span.End()

	lg := p.logger
	client := plugin.NewClient(cc)
	rpcClient, err := client.Client()
	if err != nil {
		lg.With(
			zap.Error(err),
			"plugin", md.Module,
		).Error("failed to load plugin")
		return
	}
	lg.With(
		"plugin", md.Module,
		"interfaces", lo.Keys(cc.Plugins),
	).Debug("checking if plugin implements any interfaces in the scheme")
	wg := &sync.WaitGroup{}
	for id := range cc.Plugins {
		raw, err := rpcClient.Dispense(id)
		if err != nil {
			lg.With(
				zap.Error(err),
				"plugin", md.Module,
				"id", id,
			).Debug("no implementation found")
			continue
		}
		lg.With(
			"plugin", md.Module,
			"id", id,
		).Debug("implementation found")
		switch c := rpcClient.(type) {
		case *plugin.GRPCClient:
			p.hooksMu.RLock()
			for _, h := range p.loadHooks {
				if h.hook.ShouldInvoke(raw) {
					wg.Add(1)
					h := h
					go func() {
						_, span := tracer.Start(tc, "PluginLoadHook",
							trace.WithAttributes(attribute.String("caller", h.caller)))
						defer span.End()
						defer wg.Done()
						done := h.hook.Invoke(raw, md, c.Conn)
						select {
						case <-done:
						case <-time.After(time.Second * 5):
							lg.With(
								"plugin", md.Module,
								"id", id,
								"caller", h.caller,
							).Warn("hook is taking longer than expected to complete")
							<-done
						}
					}()
				}
			}
			p.hooksMu.RUnlock()

			p.pluginsMu.Lock()
			p.activePlugins = append(p.activePlugins, activePlugin{
				md:     md,
				client: c,
				raw:    raw,
			})
			p.pluginsMu.Unlock()
		}
	}
	wg.Wait()
}

// LoadPlugins loads a set of plugins defined by the plugin configuration.
// This function loads plugins in parallel and does not block. It will invoke
// LoadingCompletedHooks once all plugins have been loaded. Once this function
// is called, it is unsafe to call LoadPlugins() or LoadOne() again for this
// plugin loader, although new hooks can still be added and will be invoked
// immediately according to the current state of the plugin loader.
func (p *PluginLoader) LoadPlugins(ctx context.Context, conf v1beta1.PluginsSpec, reattach ...*plugin.ReattachConfig) {
	tc, span := otel.Tracer("pluginloader").Start(ctx, "LoadPlugins")

	wg := &sync.WaitGroup{}
	for _, dir := range conf.Dirs {
		pluginPaths, err := plugin.Discover("plugin_*", dir)
		if err != nil {
			continue
		}
		for _, path := range pluginPaths {
			md, err := meta.ReadMetadata(path)
			if err != nil {
				p.logger.With(
					zap.String("plugin", path),
				).Error("failed to read plugin metadata", zap.Error(err))
				continue
			}
			cc := ClientConfig(md, ClientScheme, reattach...)
			wg.Add(1)
			go func() {
				defer wg.Done()
				p.LoadOne(tc, md, cc)
			}()
		}
	}
	go func() {
		defer span.End()
		wg.Wait()
		p.Complete()
	}()
}

// Complete marks the plugin loader as completed. This function will be called
// automatically by LoadPlugins(), although it can be called manually if
// LoadPlugins() is not used. It is not safe to call this function and
// LoadPlugins() on the same plugin loader. It will invoke LoadingCompletedHooks
// in parallel and does not block.
func (p *PluginLoader) Complete() {
	p.ensureNotCompleted()
	p.completed.Store(true)

	p.pluginsMu.Lock()
	numLoaded := len(p.activePlugins)
	p.pluginsMu.Unlock()

	p.hooksMu.RLock()
	for _, h := range p.completedHooks {
		go h.hook.Invoke(numLoaded)
	}
	p.hooksMu.RUnlock()
}

func (p *PluginLoader) ensureNotCompleted() {
	if p.completed.Load() {
		panic("use of PluginLoader after complete")
	}
}
