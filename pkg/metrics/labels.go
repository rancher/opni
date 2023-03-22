package metrics

import (
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/relabel"
)

const (
	LabelImpersonateAs = "_opni_impersonate_as"
)

var matchNonEmptyRegex = relabel.MustNewRegexp(".+")

// Drops metrics containing any opni internal labels.
func OpniInternalLabelFilter() *relabel.Config {
	return &relabel.Config{
		SourceLabels: model.LabelNames{
			LabelImpersonateAs,
		},
		Regex:  matchNonEmptyRegex,
		Action: relabel.Drop,
	}
}
