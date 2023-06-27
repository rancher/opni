package mock_ext

import "github.com/rancher/opni/pkg/test/testdata/plugins/ext"

type MockExtServerImpl struct {
	ext.UnsafeExtServer
	*MockExtServer
}

type MockExt2ServerImpl struct {
	ext.UnsafeExt2Server
	*MockExt2Server
}
