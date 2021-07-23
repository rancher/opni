package controllers

import (
	"time"
)

const (
	crName              = "test-opnicluster"
	crNamespace         = "opnicluster-test"
	laName              = "test-logadapter"
	laClusterName       = "test-oc-logadapter"
	laNamespace         = "logadapter-test"
	systemdLogPath      = "/var/log/testing"
	openrcLogPath       = "/var/test/alternate.log"
	openrcLogDir        = "/var/test"
	timeout             = 10 * time.Second
	consistentlyTimeout = 4 * time.Second
	interval            = 500 * time.Millisecond
)
