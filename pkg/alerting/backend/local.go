package backend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"syscall"
	"time"

	"github.com/phayes/freeport"
	"github.com/rancher/opni/pkg/alerting/config"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util/waitctx"
	"go.uber.org/zap"
)

var _ RuntimeEndpointBackend = &LocalEndpointBackend{}

var RuntimeBinaryPath = "./"

// LocalEndpointBackend implements alerting.RuntimeEndpointBackend
//
// Only used for test:env and non kubernetes environments
type LocalEndpointBackend struct {
	ConfigFilePath string
	port           int
}

func (b *LocalEndpointBackend) Start(ctx context.Context, lg *zap.SugaredLogger) {
	port, err := freeport.GetFreePort()
	fmt.Printf("AlertManager port %d", port)
	if err != nil {
		panic(err)
	}
	clusterPort, err := freeport.GetFreePort()
	if err != nil {
		panic(err)
	}
	if err != nil {
		panic(err)
	}

	//TODO: fixme relative path only works for one of tests or mage test:env, but not both
	amBin := path.Join(RuntimeBinaryPath, "bin/opni")
	defaultArgs := []string{
		"alertmanager",
		fmt.Sprintf("--config.file=%s", b.ConfigFilePath),
		fmt.Sprintf("--web.listen-address=:%d", port),
		fmt.Sprintf("--cluster.listen-address=:%d", clusterPort),
		"--storage.path=/tmp/data",
		"--log.level=debug",
	}
	cmd := exec.CommandContext(ctx, amBin, defaultArgs...)
	plugins.ConfigureSysProcAttr(cmd)
	lg.With("port", port).Info("Starting AlertManager")
	session, err := testutil.StartCmd(cmd)
	if err != nil {
		if !errors.Is(ctx.Err(), context.Canceled) {
			panic(fmt.Sprintf("%s : ambin path : %s", err, amBin))
		} else {
			return
		}
	}
	for ctx.Err() == nil {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/-/ready", port))
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(time.Second)
	}
	lg.With("address", fmt.Sprintf("http://localhost:%d", port)).Info("AlertManager started")
	waitctx.Permissive.Go(ctx, func() {
		<-ctx.Done()
		cmd, _ := session.G()
		if cmd != nil {
			cmd.Signal(os.Signal(syscall.SIGTERM))
		}
	})
	b.port = port
}

func (b *LocalEndpointBackend) Fetch(ctx context.Context, lg *zap.SugaredLogger, options shared.NewAlertingOptions, key string) (string, error) {
	data, err := os.ReadFile(b.ConfigFilePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (b *LocalEndpointBackend) Put(ctx context.Context, lg *zap.SugaredLogger, options shared.NewAlertingOptions, key string, data *config.ConfigMapData) error {
	loopError := ReconcileInvalidStateLoop(time.Duration(time.Second*10), data, lg)
	if loopError != nil {
		return shared.WithInternalServerError(fmt.Sprintf("failed to reconcile config : %s", loopError))
	}
	applyData, err := data.Marshal()
	if err != nil {
		return err
	}
	err = os.WriteFile(b.ConfigFilePath, applyData, 0644)
	return err
}

func (b *LocalEndpointBackend) Port() int {
	return b.port
}

func (b *LocalEndpointBackend) Reload(ctx context.Context, lg *zap.SugaredLogger, options shared.NewAlertingOptions, key string) error {
	retries := 10
	//if b.cancelFunc != nil {
	if b.port == 0 {
		panic("invalid port")
	}
	reload := &AlertManagerAPI{
		Endpoint: fmt.Sprintf("http://localhost:%d", b.port),
		Route:    "/-/reload",
		Verb:     POST,
	}
	webClient := &AlertManagerAPI{
		Endpoint: fmt.Sprintf("localhost:%d", b.port),
		Route:    "/-/ready",
		Verb:     GET,
	}
	lg.Info("Shutting down Alert-manager for reload...")
	resp, err := http.Post(reload.ConstructHTTP(), "application/json", nil)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode != http.StatusOK {
		panic("failed to reload alertmanager")
	}
	for i := 0; i < retries; i++ {
		resp, err := http.Get(webClient.ConstructHTTP())
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("failed to reload alertmanager")
}
