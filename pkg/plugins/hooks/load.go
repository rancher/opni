package hooks

import (
	gsync "github.com/kralicky/gpkg/sync"
	"github.com/rancher/opni/pkg/plugins/meta"
	"google.golang.org/grpc"
)

type PluginLoadHook interface {
	ShouldInvoke(t any) bool
	Invoke(t any, md meta.PluginMeta, conn *grpc.ClientConn) (done chan struct{})
}

type onLoadHook[T any] struct {
	callback onLoadCallback[T]
}

func (onLoadHook[T]) ShouldInvoke(t any) bool {
	_, ok := t.(T)
	return ok
}

func (h onLoadHook[T]) Invoke(t any, md meta.PluginMeta, cc *grpc.ClientConn) chan struct{} {
	c := make(chan struct{})
	go func(t T) {
		defer close(c)
		h.callback(t, md, cc)
	}(t.(T))
	return c
}

type onLoadCallback[T any] func(t T, md meta.PluginMeta, conn *grpc.ClientConn)

// Invokes the provided callback function when the plugin of type T is loaded.
func OnLoad[T any](fn func(T)) PluginLoadHook {
	return OnLoadMC(func(t T, _ meta.PluginMeta, _ *grpc.ClientConn) {
		fn(t)
	})
}

// Like OnLoad[T], but adds plugin metadata to the callback.
func OnLoadM[T any](fn func(T, meta.PluginMeta)) PluginLoadHook {
	return OnLoadMC(func(t T, md meta.PluginMeta, _ *grpc.ClientConn) {
		fn(t, md)
	})
}

// Like OnLoadM[T], but adds the grpc client connection to the callback.
func OnLoadMC[T any](fn func(T, meta.PluginMeta, *grpc.ClientConn)) PluginLoadHook {
	once := gsync.Map[string, bool]{}
	return &onLoadHook[T]{
		callback: func(t T, md meta.PluginMeta, cc *grpc.ClientConn) {
			if _, loaded := once.LoadOrStore(md.Module, true); !loaded {
				fn(t, md, cc)
			}
		},
	}
}
