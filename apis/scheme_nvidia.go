//go:build !minimal

package apis

import (
	opninfdv1 "github.com/rancher/opni/apis/nfd/v1"
	opninvidiav1 "github.com/rancher/opni/apis/nvidia/v1"
)

func init() {
	addSchemeBuilders(opninvidiav1.AddToScheme, opninfdv1.AddToScheme)
}
