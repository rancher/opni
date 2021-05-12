package cli

import (
	"github.com/urfave/cli"
)

type InstallConfig struct {
	KubeConfig            string
	MinioAccessKey        string
	MinioSecretKey        string
	MinioVersion          string
	NatsPassword          string
	NatsReplicas          int
	NatsMaxPayload        int
	NatsVersion           string
	NvidiaVersion         string
	ElasticsearchUser     string
	ElasticsearchPassword string
	TraefikVersion        string
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
			},
			cli.StringFlag{
				Name:        "minio-access-key",
				EnvVar:      "MINIO_ACCESS_KEY",
				Destination: &InstallCmd.MinioAccessKey,
			},
			cli.StringFlag{
				Name:        "minio-secret-key",
				EnvVar:      "MINIO_SECRET_KEY",
				Destination: &InstallCmd.MinioSecretKey,
			},
			cli.StringFlag{
				Name:        "minio-version",
				EnvVar:      "MINIO_VERSION",
				Destination: &InstallCmd.MinioVersion,
				Value:       "8.0.10",
			},
			cli.StringFlag{
				Name:        "nats-version",
				EnvVar:      "NATS_VERSION",
				Destination: &InstallCmd.NatsVersion,
				Value:       "2.2.1",
			},
			cli.StringFlag{
				Name:        "nats-password",
				EnvVar:      "NATS_PASSWORD",
				Destination: &InstallCmd.NatsPassword,
			},
			cli.IntFlag{
				Name:        "nats-replicas",
				EnvVar:      "NATS_REPLICAS",
				Destination: &InstallCmd.NatsReplicas,
				Value:       3,
			},
			cli.IntFlag{
				Name:        "nats-max-payload",
				EnvVar:      "NATS_MAX_PAYLOAD",
				Destination: &InstallCmd.NatsMaxPayload,
				Value:       10485760,
			},
			cli.StringFlag{
				Name:        "nvidia-version",
				EnvVar:      "NVIDIA_VERSION",
				Destination: &InstallCmd.NvidiaVersion,
				Value:       "1.0.0-beta6",
			},
			cli.StringFlag{
				Name:        "elasticsearch-user",
				EnvVar:      "ES_USER",
				Destination: &InstallCmd.ElasticsearchUser,
				Value:       "admin",
			},
			cli.StringFlag{
				Name:        "elasticsearch-password",
				EnvVar:      "ES_PASSWORD",
				Destination: &InstallCmd.ElasticsearchPassword,
				Value:       "admin",
			},
			cli.StringFlag{
				Name:        "traefik-version",
				EnvVar:      "TRAEFIK_VERSION",
				Destination: &InstallCmd.TraefikVersion,
				Value:       "v9.18.3",
			},
		},
	}
}
