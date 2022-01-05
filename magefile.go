//go:build mage

package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kralicky/opni-gateway/pkg/config/meta"
	"github.com/kralicky/opni-gateway/pkg/config/v1beta1"
	"github.com/kralicky/opni-gateway/pkg/management"
	"github.com/kralicky/opni-gateway/pkg/tokens"
	"github.com/kralicky/ragu/pkg/ragu"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

var Default = All
var runArgs []string

func init() {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		idx := 0
		for i, arg := range os.Args {
			if arg == "--" {
				idx = i
				break
			}
		}
		if idx == 0 {
			fmt.Println("usage: mage run -- <args>")
			os.Exit(1)
		}
		runArgs = os.Args[idx+1:]
		os.Args = os.Args[:idx]
	}
}

func All() {
	mg.SerialDeps(
		Build,
		Test,
	)
}

func Build() error {
	mg.Deps(Generate)
	return sh.RunWith(map[string]string{
		"CGO_ENABLED": "0",
	}, mg.GoCmd(), "build", "-ldflags", "-w -s", "-o", "bin/opni-gateway", "./cmd/opni-gateway")
}

func Test() error {
	return nil
}

func Run() error {
	mg.Deps(Build)
	return sh.RunV("./bin/opni-gateway", runArgs...)
}

func Docker() error {
	mg.Deps(Build)
	return sh.RunWithV(map[string]string{
		"DOCKER_BUILDKIT": "1",
	}, "docker", "build", "-t", "opni-gateway", ".")
}

func Generate() error {
	protos, err := ragu.GenerateCode("pkg/management/management.proto", true)
	if err != nil {
		return err
	}
	for _, f := range protos {
		path := filepath.Join("pkg/management", f.GetName())
		if info, err := os.Stat(path); err == nil {
			if info.Mode()&0200 == 0 {
				if err := os.Chmod(path, 0644); err != nil {
					return err
				}
			}
		}
		if err := os.WriteFile(path, []byte(f.GetContent()), 0444); err != nil {
			return err
		}
		if err := os.Chmod(path, 0444); err != nil {
			return err
		}
	}
	return nil
}

func Bootstrap() error {
	ctx := context.Background()
	c, err := management.NewClient(management.WithListenAddress("localhost:9090")) // via tilt
	if err != nil {
		return fmt.Errorf("failed to connect to the management socket (is tilt running?): %w", err)
	}
	info, err := c.CertsInfo(ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}
	last := info.Chain[len(info.Chain)-1]
	hash := last.SPKIHash

	existing, err := c.ListBootstrapTokens(ctx, &management.ListBootstrapTokensRequest{})
	if err != nil {
		return err
	}
	var token *tokens.Token
	if len(existing.Tokens) == 0 {
		t, err := c.CreateBootstrapToken(ctx, &management.CreateBootstrapTokenRequest{
			TTL: durationpb.New(1 * time.Hour),
		})
		if err != nil {
			return err
		}
		token = t.ToToken()
	} else {
		token = existing.Tokens[0].ToToken()
	}

	proxyConfig := v1beta1.ProxyConfig{
		TypeMeta: meta.TypeMeta{
			APIVersion: "v1beta1",
			Kind:       "ProxyConfig",
		},
		Spec: v1beta1.ProxyConfigSpec{
			ListenAddress:  ":8080",
			GatewayAddress: "https://opni-gateway.opni-gateway.svc.cluster.local:8080",
			IdentityProvider: v1beta1.IdentityProviderSpec{
				Type: v1beta1.IdentityProviderKubernetes,
			},
			Storage: v1beta1.StorageSpec{
				Type: v1beta1.StorageTypeSecret,
			},
			Bootstrap: v1beta1.BootstrapSpec{
				Token:      token.EncodeHex(),
				CACertHash: hex.EncodeToString(hash),
			},
		},
	}

	configData, err := yaml.Marshal(proxyConfig)
	if err != nil {
		return err
	}

	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "opni-gateway",
		},
		Data: map[string][]byte{
			"config.yaml": configData,
		},
	}

	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(), nil).ClientConfig()
	if err != nil {
		return err
	}
	clientset := kubernetes.NewForConfigOrDie(restConfig)
	_, err = clientset.CoreV1().
		Secrets("opni-gateway-proxy").
		Create(ctx, &secret, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}
