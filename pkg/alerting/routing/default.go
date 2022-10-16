package routing

import "github.com/rancher/opni/pkg/alerting/shared"

// NewDefaultRoutingTree
//
// This function cannot error, and will choose to panic instead
func NewDefaultRoutingTree(managementUrl string) *RoutingTree {
	b, err := shared.DefaultAlertManagerConfig(managementUrl)
	if err != nil {
		panic(err)
	}
	r := &RoutingTree{}
	err = r.Parse(b.String())
	if err != nil {
		panic(err)
	}
	return r
}
