package plugins

import (
	"github.com/hashicorp/go-plugin"
	"github.com/kralicky/opni-monitoring/pkg/plugins/apis/apiextensions"
	"github.com/kralicky/opni-monitoring/pkg/plugins/meta"
)

var Scheme = meta.NewScheme()

var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  plugin.CoreProtocolVersion,
	MagicCookieKey:   "OPNI_MONITORING_MAGIC_COOKIE",
	MagicCookieValue: "opni-monitoring",
}

func init() {
	apiextensions.AddToScheme(Scheme)
}
