//go:build !minimal

package apis

import opensearchv1 "github.com/Opster/opensearch-k8s-operator/opensearch-operator/api/v1"

func init() {
	addSchemeBuilders(opensearchv1.AddToScheme)
}
