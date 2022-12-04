package types

type DashboardsVersionDoc struct {
	DashboardVersion string `json:"version"`
}

type UpsertDashboardsDoc struct {
	Document         DashboardsVersionDoc `json:"doc,omitempty"`
	DocumentAsUpsert *bool                `json:"doc_as_upsert,omitempty"`
}

type DashboardsDocResponse struct {
	Index       string               `json:"_index,omitempty"`
	ID          string               `json:"_id,omitempty"`
	SeqNo       int                  `json:"_seq_no,omitempty"`
	PrimaryTerm int                  `json:"_primary_term,omitempty"`
	Found       *bool                `json:"found,omitempty"`
	Source      DashboardsVersionDoc `json:"_source,omitempty"`
}
