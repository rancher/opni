package testk8s

import (
	"context"
	"fmt"
	"time"

	k8sruntime "k8s.io/apimachinery/pkg/runtime"

	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/freeport"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/waitctx"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func StartManager(ctx waitctx.PermissiveContext, restConfig *rest.Config, scheme *k8sruntime.Scheme, reconcilers ...Reconciler) ctrl.Manager {
	ports := freeport.GetFreePorts(2)

	manager := util.Must(ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     fmt.Sprintf(":%d", ports[0]),
		HealthProbeBindAddress: fmt.Sprintf(":%d", ports[1]),
	}))
	for _, reconciler := range reconcilers {
		util.Must(reconciler.SetupWithManager(manager))
	}
	waitctx.Permissive.Go(ctx, func() {
		if err := manager.Start(ctx); err != nil {
			panic(err)
		}
	})
	return manager
}

func StartK8s(ctx waitctx.PermissiveContext, crdDirs []string) (*rest.Config, *k8sruntime.Scheme, error) {
	port := freeport.GetFreePort()

	scheme := apis.NewScheme()

	testbin, err := test.FindTestBin()
	if err != nil {
		return nil, nil, err
	}

	k8sEnv := &envtest.Environment{
		BinaryAssetsDirectory: testbin,
		Scheme:                scheme,
		CRDDirectoryPaths:     crdDirs,
		CRDs:                  GetCertManagerCRDs(scheme),
		ErrorIfCRDPathMissing: true,
		ControlPlane: envtest.ControlPlane{
			APIServer: &envtest.APIServer{
				SecureServing: envtest.SecureServing{
					ListenAddr: envtest.ListenAddr{
						Address: "127.0.0.1",
						Port:    fmt.Sprint(port),
					},
				},
			},
		},
	}

	cfg, err := k8sEnv.Start()
	if err != nil {
		return nil, nil, err
	}

	// wait for the apiserver to be ready
	readyCount := 0
	client := kubernetes.NewForConfigOrDie(cfg).CoreV1().RESTClient().Get().AbsPath("/healthz")
	for readyCount < 5 {
		response := client.Do(context.Background())
		if response.Error() == nil {
			var code int
			response.StatusCode(&code)
			if code == 200 {
				readyCount++
				continue
			}
		}
		readyCount = 0
		time.Sleep(100 * time.Millisecond)
	}

	waitctx.Permissive.Go(ctx, func() {
		<-ctx.Done()
		k8sEnv.Stop()
	})

	return cfg, scheme, nil
}

type Reconciler interface {
	SetupWithManager(ctrl.Manager) error
}
