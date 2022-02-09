package types

type KibanaVersionDoc struct {
	DashboardVersion string `json:"version"`
}

type UpsertKibanaDoc struct {
	Document         KibanaVersionDoc `json:"doc,omitempty"`
	DocumentAsUpsert *bool            `json:"doc_as_upsert,omitempty"`
}

type KibanaDocResponse struct {
	Index       string           `json:"_index,omitempty"`
	ID          string           `json:"_id,omitempty"`
	SeqNo       int              `json:"_seq_no,omitempty"`
	PrimaryTerm int              `json:"_primary_term,omitempty"`
	Found       *bool            `json:"found,omitempty"`
	Source      KibanaVersionDoc `json:"_source,omitempty"`
}
