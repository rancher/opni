package types

type ReindexSpec struct {
	Source      ReindexIndexSpec `json:"source"`
	Destination ReindexIndexSpec `json:"dest"`
}

type ReindexIndexSpec struct {
	Index string `json:"index,omitempty"`
}
