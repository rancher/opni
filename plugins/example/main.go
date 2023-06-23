package main

import (
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/meta"
	_ "github.com/rancher/opni/pkg/storage/etcd"
	_ "github.com/rancher/opni/pkg/storage/jetstream"
	"github.com/rancher/opni/plugins/example/pkg/example"
)

func main() {
	m := plugins.Main{
		Modes: meta.ModeSet{
			meta.ModeGateway: example.Scheme,
			meta.ModeAgent:   example.Scheme,
		},
	}
	m.Exec()
}
