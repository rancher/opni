package cli

import (
	"github.com/urfave/cli"
)

type InstallConfig struct {
	KubeConfig             string
	MinioAccessKey         string
	MinioSecretKey         string
	MinioVersion           string
	NatsPassword           string
	NatsReplicas           int
	NatsMaxPayload         int
	NatsVersion            string
	NvidiaVersion          string
	ElasticsearchUser      string
	ElasticsearchPassword  string
	TraefikVersion         string
	NulogServiceCPURequest string
	Disable                cli.StringSlice
	QuickStart             bool
}

var InstallCmd InstallConfig

func NewInstallCommand(action func(*cli.Context) error) cli.Command {
	return cli.Command{
		Name:      "install",
		Usage:     "install opni stack",
		UsageText: "opnictl install [OPTIONS]",
		Action:    action,
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:        "kubeconfig",
				EnvVar:      "KUBECONFIG",
				Destination: &InstallCmd.KubeConfig,
				Usage:       "Kubeconfig file to access the kubernetes cluster",
				Value:       "~/.kube/config",
			},
			cli.StringFlag{
				Name:        "minio-access-key",
				EnvVar:      "MINIO_ACCESS_KEY",
				Destination: &InstallCmd.MinioAccessKey,
				Usage:       "Minio access key, if empty a randomized string will be used",
			},
			cli.StringFlag{
				Name:        "minio-secret-key",
				EnvVar:      "MINIO_SECRET_KEY",
				Destination: &InstallCmd.MinioSecretKey,
				Usage:       "Minio secret key, if empty a randomized string will be used",
			},
			cli.StringFlag{
				Name:        "minio-version",
				EnvVar:      "MINIO_VERSION",
				Destination: &InstallCmd.MinioVersion,
				Value:       "8.0.10",
				Usage:       "Minio chart version",
			},
			cli.StringFlag{
				Name:        "nats-version",
				EnvVar:      "NATS_VERSION",
				Destination: &InstallCmd.NatsVersion,
				Value:       "2.2.1",
				Usage:       "Nats chart version",
			},
			cli.StringFlag{
				Name:        "nats-password",
				EnvVar:      "NATS_PASSWORD",
				Destination: &InstallCmd.NatsPassword,
				Usage:       "Nats Password, if empty a randomized string will be used",
			},
			cli.IntFlag{
				Name:        "nats-replicas",
				EnvVar:      "NATS_REPLICAS",
				Destination: &InstallCmd.NatsReplicas,
				Value:       3,
				Usage:       "Nats pods replicas",
			},
			cli.IntFlag{
				Name:        "nats-max-payload",
				EnvVar:      "NATS_MAX_PAYLOAD",
				Destination: &InstallCmd.NatsMaxPayload,
				Value:       10485760,
				Usage:       "Nats maximum payload",
			},
			cli.StringFlag{
				Name:        "nvidia-version",
				EnvVar:      "NVIDIA_VERSION",
				Destination: &InstallCmd.NvidiaVersion,
				Value:       "1.0.0-beta6",
				Usage:       "Nvidia plugin version",
			},
			cli.StringFlag{
				Name:        "elasticsearch-user",
				EnvVar:      "ES_USER",
				Destination: &InstallCmd.ElasticsearchUser,
				Value:       "admin",
				Usage:       "Elasticsearch username",
			},
			cli.StringFlag{
				Name:        "elasticsearch-password",
				EnvVar:      "ES_PASSWORD",
				Destination: &InstallCmd.ElasticsearchPassword,
				Value:       "admin",
				Usage:       "Elasticsearch password",
			},
			cli.StringFlag{
				Name:        "traefik-version",
				EnvVar:      "TRAEFIK_VERSION",
				Destination: &InstallCmd.TraefikVersion,
				Value:       "v9.18.3",
				Usage:       "Traefik chart version",
			},
			cli.StringFlag{
				Name:        "nulog-service-cpu-request",
				EnvVar:      "NULOG_SERVICE_CPU_REQUEST",
				Destination: &InstallCmd.NulogServiceCPURequest,
				Usage:       "CPU resource request for nulog service controlplane",
			},
			cli.StringSliceFlag{
				Name:  "disable",
				Value: &InstallCmd.Disable,
				Usage: "Disable list, available values are (helm-controller, local-path-provisioner, minio, nats, opendistro-es, traefik, rancher-logging, drain-service, log-outpout, nulog-inference-service, nulog-inference-service-control-plane, nvidia-plugin, payload-receiver-service, preprocessing, training-controller)",
			},
			cli.BoolFlag{
				Name:        "quickstart",
				Destination: &InstallCmd.QuickStart,
				Usage:       "Quickstart mode",
			},
		},
	}
}
