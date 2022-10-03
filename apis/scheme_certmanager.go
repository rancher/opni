//go:build !noscheme_certmanager && !noscheme_thirdparty

package apis

import cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"

func init() {
	addSchemeBuilders(cmv1.AddToScheme)
}
