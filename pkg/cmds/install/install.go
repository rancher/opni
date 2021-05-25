package install

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	cmds "github.com/rancher/opnictl/pkg/cmds/cli"
	"github.com/rancher/opnictl/pkg/deploy"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func Run(c *cli.Context) error {
	logrus.Info("Starting installer")
	rand.Seed(time.Now().UTC().UnixNano())
	ctx := context.Background()

	cfg := cmds.InstallCmd

	sc, err := deploy.NewContext(ctx, cfg.KubeConfig)
	if err != nil {
		return err
	}

	values, err := getValues(ctx, &cfg, sc)
	if err != nil {
		return err
	}

	disabled, err := getDisabledList(ctx, &cfg, sc)
	if err != nil {
		return err
	}

	if err := sc.Start(ctx); err != nil {
		return err
	}

	if err := deploy.Install(ctx, sc, values, disabled); err != nil {
		return err
	}
	return nil
}

func getValues(ctx context.Context, cfg *cmds.InstallConfig, sc *deploy.Context) (map[string]string, error) {
	// try getting first from configMap
	cfgSecret, err := sc.K8s.CoreV1().Secrets(deploy.OpniSystemNS).Get(ctx, deploy.OpniConfig, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if cfgSecret != nil && len(cfgSecret.Data) > 0 {
		cfg.MinioAccessKey = string(cfgSecret.Data[deploy.MinioAccessKey])
		cfg.MinioSecretKey = string(cfgSecret.Data[deploy.MinioSecretKey])
		cfg.NatsPassword = string(cfgSecret.Data[deploy.NatsPassword])
		cfg.ElasticsearchPassword = string(cfgSecret.Data[deploy.ESPassword])
	}
	values := make(map[string]string)
	// get minio values
	values[deploy.MinioAccessKey] = cfg.MinioAccessKey
	values[deploy.MinioSecretKey] = cfg.MinioSecretKey
	values[deploy.MinioVersion] = cfg.MinioVersion
	if cfg.MinioAccessKey == "" || cfg.MinioSecretKey == "" {
		minioAccessKey := randStringRunes(8)
		minioSecretKey := randStringRunes(8)
		values[deploy.MinioAccessKey] = string(minioAccessKey)
		values[deploy.MinioSecretKey] = string(minioSecretKey)
	}

	// get nats values
	values[deploy.NatsPassword] = cfg.NatsPassword
	if cfg.NatsPassword == "" {
		natsPassword := randStringRunes(8)
		values[deploy.NatsPassword] = string(natsPassword)
	}
	values[deploy.NatsMaxPayload] = strconv.Itoa(cfg.NatsMaxPayload)
	values[deploy.NatsReplicas] = strconv.Itoa(cfg.NatsReplicas)
	values[deploy.NatsVersion] = cfg.NatsVersion

	// get nvidia values
	values[deploy.NvidiaVersion] = cfg.NvidiaVersion

	// get elastic search values
	values[deploy.ESUser] = cfg.ElasticsearchUser
	if cfg.ElasticsearchUser == "" {
		values[deploy.ESUser] = "admin"
	}
	values[deploy.ESPassword] = cfg.ElasticsearchPassword
	if cfg.ElasticsearchPassword == "" {
		esPass := randStringRunes(8)
		values[deploy.ESPassword] = string(esPass)
	}

	// get nulog values

	values[deploy.NulogServiceCPURequest] = cfg.NulogServiceCPURequest
	if values[deploy.NulogServiceCPURequest] == "" {
		// overriding nulog service cpu request when no value is passed
		values[deploy.NulogServiceCPURequest] = "3.5"
		if cfg.QuickStart {
			values[deploy.NulogServiceCPURequest] = "1"
		}

	}

	// get traefik values
	values[deploy.TraefikVersion] = cfg.TraefikVersion
	return values, nil
}

func getDisabledList(ctx context.Context, cfg *cmds.InstallConfig, sc *deploy.Context) ([]string, error) {
	var (
		isRKE2 bool
		isK3S  bool
	)
	// check cluster type
	nodes, err := sc.Core.Core().V1().Node().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(nodes.Items) <= 0 {
		return nil, fmt.Errorf("Empty node list")
	}
	if strings.Contains(nodes.Items[0].Spec.ProviderID, "k3s") {
		isK3S = true
	} else if strings.Contains(nodes.Items[0].Spec.ProviderID, "rke2") {
		isRKE2 = true
	}

	if isRKE2 || isK3S {
		// disable helm controller if rke2 or k3s
		cfg.Disable = append(cfg.Disable, "helm-controller")
	}
	if isK3S {
		// disable local path provisioner in k3s only
		cfg.Disable = append(cfg.Disable, "local-path-provisioner")
	}
	if !cfg.QuickStart {
		// disable rancher logging if quick start flag is not passed
		cfg.Disable = append(cfg.Disable, "rancher-logging", "log-output")
	} else {
		// disable nulog service, nvidia plugin and training controller in quick start mode
		cfg.Disable = append(cfg.Disable, "nulog-inference-service.yaml", "nvidia-plugin", "training-controller")
	}

	return cfg.Disable, nil
}
