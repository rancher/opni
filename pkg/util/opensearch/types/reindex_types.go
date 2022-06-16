package types

type ReindexSpec struct {
	Source      ReindexSourceSpec `json:"source"`
	Destination ReindexDestSpec   `json:"dest"`
}

type ReindexSourceSpec struct {
	Index []string `json:"index,omitempty"`
}

type ReindexDestSpec struct {
	Index string `json:"index,omitempty"`
}
