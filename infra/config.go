package main

import (
	"os"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func LoadConfig(ctx *pulumi.Context) *Config {
	var clusterConfig ClusterConfig
	namePrefix := config.Require(ctx, "opni:namePrefix")
	zoneID := config.Require(ctx, "opni:zoneID")
	useLocalCharts := config.GetBool(ctx, "opni:useLocalCharts")
	config.GetObject(ctx, "opni:cluster", &clusterConfig)
	tags := map[string]string{}
	config.GetObject(ctx, "opni:tags", &tags)
	useIdInDnsNames := config.GetBool(ctx, "opni:useIdInDnsNames")
	clusterConfig.LoadDefaults()

	var cloud, imageRepo, imageTag, minimalImageTag string

	if value, ok := os.LookupEnv("CLOUD"); ok {
		cloud = value
	} else {
		cloud = config.Get(ctx, "opni:cloud")
	}
	if value, ok := os.LookupEnv("IMAGE_REPO"); ok {
		imageRepo = value
	} else {
		imageRepo = config.Get(ctx, "opni:imageRepo")
	}
	if value, ok := os.LookupEnv("IMAGE_TAG"); ok {
		imageTag = value
	} else {
		imageTag = config.Get(ctx, "opni:imageTag")
	}
	if value, ok := os.LookupEnv("MINIMAL_IMAGE_TAG"); ok {
		minimalImageTag = value
	} else {
		minimalImageTag = config.Get(ctx, "opni:minimalImageTag")
	}

	conf := &Config{
		NamePrefix:      namePrefix,
		ZoneID:          zoneID,
		Cloud:           cloud,
		ImageRepo:       imageRepo,
		ImageTag:        imageTag,
		MinimalImageTag: minimalImageTag,
		UseLocalCharts:  useLocalCharts,
		Cluster:         clusterConfig,
		Tags:            tags,
		UseIdInDnsNames: useIdInDnsNames,
	}
	conf.LoadDefaults()
	return conf
}

type Config struct {
	NamePrefix            string            `json:"namePrefix"`
	ZoneID                string            `json:"zoneID"`
	Cloud                 string            `json:"cloud"`
	ImageRepo             string            `json:"imageRepo"`
	ImageTag              string            `json:"imageTag"`
	MinimalImageTag       string            `json:"minimalImageTag"`
	UseLocalCharts        bool              `json:"useLocalCharts"`
	ChartsRepo            string            `json:"chartsRepo"`
	ChartVersion          string            `json:"chartVersion"`
	KubePrometheusEnabled bool              `json:"kubePrometheusEnabled"`
	Cluster               ClusterConfig     `json:"cluster"`
	Tags                  map[string]string `json:"tags"`
	UseIdInDnsNames       bool              `json:"useIdInDnsNames"`
}

type ClusterConfig struct {
	NodeInstanceType     string `json:"nodeInstanceType"`
	NodeGroupMinSize     int    `json:"nodeGroupMinSize"`
	NodeGroupMaxSize     int    `json:"nodeGroupMaxSize"`
	NodeGroupDesiredSize int    `json:"nodeGroupDesiredSize"`
}

func (c *Config) LoadDefaults() {
	if c.Cloud == "" {
		c.Cloud = "aws"
	}
	if c.ImageRepo == "" {
		c.ImageRepo = "rancher/opni"
	}
	if c.ImageTag == "" {
		c.ImageTag = "latest"
	}
	if c.MinimalImageTag == "" {
		c.MinimalImageTag = "latest-minimal"
	}
	if c.ChartsRepo == "" {
		c.ChartsRepo = "https://raw.githubusercontent.com/rancher/opni/charts-repo/"
	}
	c.Cluster.LoadDefaults()
}

func (c *ClusterConfig) LoadDefaults() {
	if c.NodeInstanceType == "" {
		c.NodeInstanceType = "r6a.xlarge"
	}
	if c.NodeGroupMinSize == 0 {
		c.NodeGroupMinSize = 3
	}
	if c.NodeGroupMaxSize == 0 {
		c.NodeGroupMaxSize = 3
	}
	if c.NodeGroupDesiredSize == 0 {
		c.NodeGroupDesiredSize = 3
	}
}
