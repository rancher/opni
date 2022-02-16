package plugins

import (
	"os"
	"os/signal"

	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni-monitoring/pkg/plugins/meta"
)

var Scheme = meta.NewScheme()

var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  plugin.CoreProtocolVersion,
	MagicCookieKey:   "OPNI_MONITORING_MAGIC_COOKIE",
	MagicCookieValue: "opni-monitoring",
}

func init() {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt)

	go func() {
		<-c
		plugin.CleanupClients()
		os.Exit(0)
	}()
}
