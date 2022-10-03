//go:build !noscheme_opensearch && !noscheme_thirdparty

package apis

import opensearchv1 "opensearch.opster.io/api/v1"

func init() {
	addSchemeBuilders(opensearchv1.AddToScheme)
}
