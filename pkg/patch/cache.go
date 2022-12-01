package patch

import (
	"github.com/prometheus/client_golang/prometheus"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
)

type Cache interface {
	// RequestPatch will return a patch which will transform the plugin at oldHash to the plugin at newHash. If the patch
	// does not exist, and both referenced plugins exist, the patch will be generated and stored in the cache before
	// returning.
	// This function is thread-safe and reentrant. The patch will only be computed once regardless of how many
	// concurrent calls are made.
	// If the patch does not exist, and one or both of the referenced plugins do not exist, this function will return
	// an error, and the patch will not be generated.
	RequestPatch(oldHash, newHash string) ([]byte, error)

	// PatchKey Returns an opaque unique key representing a patch between the two revisions. The key is guaranteed to be
	// unique for the inputs but is not guaranteed to have a specific format, or even contain the input strings.
	PatchKey(oldHash, newHash string) string

	// Archive compresses and saves all plugins in the given plugin manifest to the cache, if they do not exist.
	Archive(manifest *controlv1.PluginManifest) error

	// GetPlugin returns the plugin with the given hash, if it exists.
	GetPlugin(hash string) ([]byte, error)

	// ListDigests returns a list of all plugin digests in the cache. It does not return patch digests.
	ListDigests() ([]string, error)

	// Clean removes all objects in the cache associated with the given plugin hashes, including any patches that
	// reference those plugins.
	Clean(hashes ...string)

	// MetricsCollectors returns a list of prometheus collectors that can be used to track cache metrics.
	MetricsCollectors() []prometheus.Collector

	// MetricsSnapshot returns a snapshot of the current cache metrics.
	MetricsSnapshot() CacheMetrics
}
