//go:build !minimal

package apis

import opensearchv1 "opensearch.opster.io/api/v1"

func init() {
	addSchemeBuilders(opensearchv1.AddToScheme)
}
