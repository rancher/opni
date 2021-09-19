package types

type UpdateAliasRequest struct {
	Actions []AliasActionSpec `json:"actions,omitempty"`
}

type AliasActionSpec struct {
	*AliasAtomicAction `json:",inline,omitempty"`
}

type AliasAtomicAction struct {
	Add         *AliasGenericAction `json:"add,omitempty"`
	Remove      *AliasGenericAction `json:"remove,omitempty"`
	RemoveIndex *AliasGenericAction `json:"remove_index,omitempty"`
}

type AliasGenericAction struct {
	Alias         string                 `json:"alias,omitempty"`
	Aliases       []string               `json:"aliases,omitempty"`
	Filter        map[string]interface{} `json:"filter,omitempty"`
	Index         string                 `json:"index,omitempty"`
	Indices       []string               `json:"indices,omitempty"`
	IndexRouting  string                 `json:"index_routing,omitempty"`
	IsHidden      *bool                  `json:"is_hidden,omitempty"`
	IsWriteIndex  *bool                  `json:"is_write_index,omitempty"`
	MustExist     *bool                  `json:"must_exist,omitempty"`
	Routing       string                 `json:"routing,omitempty"`
	SearchRouting string                 `json:"search_routing,omitempty"`
}
