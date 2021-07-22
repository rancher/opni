package controllers

import (
	"time"
)

const (
	crName        = "test-opnicluster"
	crNamespace   = "opnicluster-test"
	laName        = "test-logadapter"
	laClusterName = "test-oc-logadapter"
	laNamespace   = "logadapter-test"
	rke2LogPath   = "/var/log/testing"
	timeout       = 10 * time.Second
	interval      = 500 * time.Millisecond
)
