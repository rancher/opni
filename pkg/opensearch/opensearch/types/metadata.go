package types

type ClusterMetadataDocUpdate struct {
	Name string `json:"name,omitempty"`
}

// ClusterMetadataDoc defines the available metadata for each logging data.
type ClusterMetadataDoc struct {
	ClusterMetadataDocUpdate

	Id string `json:"id,omitempty"`
}

type MetadataUpdate struct {
	Document ClusterMetadataDocUpdate `json:"doc,omitempty"`
}

type ClusterMetadataDocResponse struct {
	Index       string             `json:"_index,omitempty"`
	ID          string             `json:"_id,omitempty"`
	SeqNo       int                `json:"_seq_no,omitempty"`
	PrimaryTerm int                `json:"_primary_term,omitempty"`
	Found       *bool              `json:"found,omitempty"`
	Source      ClusterMetadataDoc `json:"_source,omitempty"`
}
