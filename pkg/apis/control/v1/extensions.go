package v1

import (
	"encoding/hex"
	"hash"

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

func (m *PluginManifestEntry) DigestBytes() []byte {
	decoded, _ := hex.DecodeString(m.GetDigest())
	return decoded
}

func (m *PluginManifestEntry) DigestHash() hash.Hash {
	h, _ := blake2b.New256(nil)
	return h
}

func (m *PluginManifest) Sort() {
	slices.SortFunc(m.Items, func(a, b *PluginManifestEntry) bool {
		return a.GetModule() < b.GetModule()
	})
}

func (m *PluginArchive) Sort() {
	slices.SortFunc(m.Items, func(a, b *PluginArchiveEntry) bool {
		return a.GetMetadata().GetModule() < b.GetMetadata().GetModule()
	})
}

func (a *PluginArchive) ToManifest() *PluginManifest {
	manifest := &PluginManifest{}
	for _, entry := range a.Items {
		manifest.Items = append(manifest.Items, entry.Metadata)
	}
	return manifest
}

func (a *PatchList) Sort() {
	slices.SortFunc(a.Items, func(a, b *PatchSpec) bool {
		if a.GetOp() != b.GetOp() {
			return a.GetOp() < b.GetOp()
		}
		if a.GetModule() != b.GetModule() {
			return a.GetModule() < b.GetModule()
		}
		return a.GetFilename() < b.GetFilename()
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
	for _, entry := range m.GetItems() {
		hash.Write([]byte(entry.GetModule()))
		hash.Write([]byte(entry.GetFilename()))
		hash.Write([]byte(entry.GetDigest()))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
