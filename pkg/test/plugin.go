package test

import (
	"context"
	"crypto/tls"
	"os"
	"regexp"
	"runtime"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/test/testdata"
	"github.com/rancher/opni/pkg/util"
	"google.golang.org/grpc"
)

type apiextensionTestPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	server  apiextensions.ManagementAPIExtensionServer
	svcDesc *grpc.ServiceDesc
	impl    interface{}
}

func (p *apiextensionTestPlugin) GRPCServer(
	_ *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	s.RegisterService(p.svcDesc, p.impl)
	apiextensions.RegisterManagementAPIExtensionServer(s, p.server)
	return nil
}

func (p *apiextensionTestPlugin) GRPCClient(
	_ context.Context,
	_ *plugin.GRPCBroker,
	c *grpc.ClientConn,
) (interface{}, error) {
	return apiextensions.NewManagementAPIExtensionClient(c), nil
}

func NewApiExtensionTestPlugin(
	srv apiextensions.ManagementAPIExtensionServer,
	svcDesc *grpc.ServiceDesc,
	impl interface{},
) *plugin.ClientConfig {
	p := &apiextensionTestPlugin{
		server:  srv,
		svcDesc: svcDesc,
		impl:    impl,
	}
	scheme := meta.NewScheme()
	scheme.Add(managementext.ManagementAPIExtensionPluginID, p)

	cert, caPool, err := util.LoadServingCertBundle(v1beta1.CertsSpec{
		CACertData:      testdata.TestData("root_ca.crt"),
		ServingCertData: testdata.TestData("localhost.crt"),
		ServingKeyData:  testdata.TestData("localhost.key"),
	})
	if err != nil {
		panic(err)
	}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*cert},
		RootCAs:      caPool,
		ServerName:   "localhost",
	}

	cfg := plugins.ServeConfig(scheme)
	ch := make(chan *plugin.ReattachConfig)
	cfg.Test = &plugin.ServeTestConfig{
		ReattachConfigCh: ch,
	}
	cfg.TLSProvider = func() (*tls.Config, error) {
		return tlsConfig, nil
	}
	go plugin.Serve(cfg)

	return &plugin.ClientConfig{
		HandshakeConfig: plugins.Handshake,
		Plugins:         scheme.PluginMap(),
		Reattach:        <-ch,
		Managed:         true,
		Logger: hclog.New(&hclog.LoggerOptions{
			Level: hclog.Error,
		}),
		Stderr:    os.Stderr,
		TLSConfig: tlsConfig,
	}
}

type testPlugin struct {
	SchemeFunc func(context.Context) meta.Scheme
	Metadata   meta.PluginMeta
}

type TestPluginSet map[meta.PluginMode][]testPlugin

var globalTestPlugins = &TestPluginSet{}

func (tp TestPluginSet) LoadPlugins(ctx context.Context, loader *plugins.PluginLoader, mode meta.PluginMode) int {
	cert, caPool, err := util.LoadServingCertBundle(v1beta1.CertsSpec{
		CACertData:      testdata.TestData("root_ca.crt"),
		ServingCertData: testdata.TestData("localhost.crt"),
		ServingKeyData:  testdata.TestData("localhost.key"),
	})
	if err != nil {
		panic(err)
	}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*cert},
		RootCAs:      caPool,
		ServerName:   "localhost",
	}

	wg := &sync.WaitGroup{}
	for _, p := range tp[mode] {
		p := p
		scheme := p.SchemeFunc(ctx)
		sc := plugins.ServeConfig(scheme)
		ch := make(chan *plugin.ReattachConfig, 1)
		sc.Test = &plugin.ServeTestConfig{
			ReattachConfigCh: ch,
		}
		sc.TLSProvider = func() (*tls.Config, error) {
			return tlsConfig, nil
		}
		go plugin.Serve(sc)
		cc := plugins.ClientConfig(p.Metadata, scheme, plugins.WithReattachConfig(<-ch))
		cc.TLSConfig = tlsConfig
		wg.Add(1)
		go func() {
			defer wg.Done()
			loader.LoadOne(ctx, p.Metadata, cc)
		}()
	}
	wg.Wait()
	loader.Complete()
	return len(tp)
}

func (tp TestPluginSet) EnablePlugin(pkgName, pluginName string, mode meta.PluginMode, schemeFunc func(context.Context) meta.Scheme) {
	for _, m := range tp[mode] {
		if m.Metadata.BinaryPath == pluginName {
			panic("bug: duplicate plugin name: " + pluginName)
		}
	}
	tp[mode] = append(tp[mode], testPlugin{
		SchemeFunc: schemeFunc,
		Metadata: meta.PluginMeta{
			BinaryPath: pluginName,
			GoVersion:  runtime.Version(),
			Module:     pkgName,
		},
	})
}

// Adds the calling plugin to the list of plugins that will be loaded
// in the test environment. The plugin metadata is inferred from the
// caller's package. This will apply to all test environments in a suite.
//
// Must be called from init() in a package of the form
// github.com/rancher/opni/plugins/<name>/test
func EnablePlugin(mode meta.PluginMode, schemeFunc func(context.Context) meta.Scheme) {
	// Get the top-level plugin package name, e.g. "github.com/rancher/opni/plugins/example"
	pc, _, _, ok := runtime.Caller(1)
	if !ok {
		panic("failed to get caller")
	}

	fn := runtime.FuncForPC(pc)
	name := fn.Name() // "github.com/rancher/opni/plugins/<name>/test.init.x"

	regex := regexp.MustCompile(`^github.com/rancher/opni/plugins/(\w+)/test.init.\d+$`)

	matches := regex.FindStringSubmatch(name)
	if len(matches) != 2 {
		panic("EnablePlugin must be called from init (caller: " + name + ")")
	}

	pkgName := "github.com/rancher/opni/plugins/" + matches[1]
	pluginName := "plugin_" + matches[1]

	globalTestPlugins.EnablePlugin(pkgName, pluginName, mode, schemeFunc)
}
