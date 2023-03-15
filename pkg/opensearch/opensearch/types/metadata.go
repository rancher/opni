package types

type ClusterMetadataUpdate struct {
	Name string `json:"name,omitempty"`
}

type MetadataUpdate struct {
	Document ClusterMetadataUpdate `json:"doc,omitempty"`
}
