package v1

import (
	"encoding/hex"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/exp/slices"
)

func (m *PluginManifest) DigestSet() map[string]struct{} {
	hm := map[string]struct{}{}
	for _, v := range m.Items {
		hm[v.GetDigest()] = struct{}{}
	}
	return hm
}

func (m *PluginManifest) PluginDigests() map[string]string {
	hm := map[string]string{}
	for _, v := range m.Items {
		hm[v.GetId()] = v.GetDigest()
	}
	return hm
}

func (m *PluginManifestEntry) GetId() string {
	return m.GetModule()
}

func (m *PluginManifest) Sort() {
	slices.SortFunc(m.Items, func(a, b *PluginManifestEntry) bool {
		return a.Module < b.Module
	})
}

func (a *PluginArchive) Sort() {
	slices.SortFunc(a.Items, func(a, b *PluginArchiveEntry) bool {
		return a.Op < b.Op && a.Module < b.Module
	})
}

const (
	ManifestDigestKey = "manifest-digest"
	AgentBuildInfoKey = "agent-build-info"
)

// Returns a hash of the manifest metadata list. This can be used to compare
// manifests between the gateway and agent.
func (m *PluginManifest) Digest() string {
	m.Sort()
	hash, _ := blake2b.New256(nil)
	for _, entry := range m.Items {
		hash.Write([]byte(entry.Module))
		hash.Write([]byte(entry.ShortName))
		hash.Write([]byte(entry.Digest))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
