package test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/phayes/freeport"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var defaultControllers = []string{
	"cronjob",
	"daemonset",
	"deployment",
	"garbagecollector",
	"namespace",
	"replicaset",
	"service",
	"serviceaccount",
	"statefulset",
}

func StartControllerManager(ctx context.Context, testEnv *envtest.Environment) {
	cfg := testEnv.Config
	controllerMgrBin := path.Join(testEnv.BinaryAssetsDirectory, "kube-controller-manager")

	apiCfg := api.Config{
		Clusters: map[string]*api.Cluster{
			"default": {
				Server:                   cfg.Host,
				CertificateAuthorityData: cfg.CAData,
			},
		},
		Contexts: map[string]*api.Context{
			"default": {
				Cluster:  "default",
				AuthInfo: "default",
			},
		},
		AuthInfos: map[string]*api.AuthInfo{
			"default": {
				ClientCertificateData: cfg.CertData,
				ClientKeyData:         cfg.KeyData,
				Token:                 cfg.BearerToken,
				Username:              cfg.Username,
				Password:              cfg.Password,
			},
		},
		CurrentContext: "default",
	}
	port, err := freeport.GetFreePort()
	if err != nil {
		panic(err)
	}
	clientcmd.WriteToFile(apiCfg, path.Join(testEnv.BinaryAssetsDirectory, "kubeconfig.yaml"))
	cmd := exec.CommandContext(ctx, controllerMgrBin,
		"--kubeconfig", path.Join(testEnv.BinaryAssetsDirectory, "kubeconfig.yaml"),
		"--controllers", strings.Join(defaultControllers, ","),
		"--leader-elect=false",
		"--address=127.0.0.1",
		fmt.Sprintf("--port=%d", port),
		"--secure-port=0",
		"--enable-garbage-collector",
		"--concurrent-gc-syncs=40",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	go func() {
		err := cmd.Run()
		if err != nil {
			if ctx.Err() == nil {
				panic(err)
			} else {
				fmt.Fprintln(os.Stderr, err)
			}
		}
	}()
}
