// Package hooks contains interfaces used to invoke callbacks at specific
// points during the plugin loading process.
//
// Hook types:
//
// # PluginLoadHook
//
// This hook is invoked whenever a plugin is loaded by the plugin loader.
//
// Use the OnLoad* methods to construct a new PluginLoadHook with the type
// of the plugin you want to be notified for. Known plugin types can be
// found in pkg/plugins/types.
//
// Load hooks will be invoked exactly once per plugin, per hook, in a separate
// goroutine. All load hooks for a particular event are run in parallel.
//
// Load hooks should not block for an extended period of time. When a plugin
// is loaded, it will block until all hooks have completed (returned). Blocking
// inside a hook can cause delays in the loading process or deadlock. Hooks
// can be registered during other hook callbacks, but take care to avoid
// deadlocks.
//
// # LoadingCompletedHook
//
// This hook is invoked after all plugins have been loaded, which occurs when
// all load hooks for all plugins have completed (returned).
//
// Use the OnLoadingCompleted method to construct a new LoadingCompletedHook.
//
// There are no restrictions related to blocking in an OnLoadingCompleted hook
// callback. It is valid to block indefinitely, e.g. to start a server.
package hooks
