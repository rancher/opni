package manager

import (
	"github.com/go-logr/logr"
	upgraderesponder "github.com/longhorn/upgrade-responder/client"
	"golang.org/x/mod/semver"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	VersionTagLatest = "latest"
)

type UpgradeRequester struct {
	Version string
	log     logr.Logger
}

func (r *UpgradeRequester) SetupLoggerWithManager(mgr ctrl.Manager) {
	r.log = mgr.GetLogger().WithName("upgrade-check")
}

func (r *UpgradeRequester) GetCurrentVersion() string {
	return r.Version
}

func (r *UpgradeRequester) GetExtraInfo() map[string]string {
	return map[string]string{}
}

func (r *UpgradeRequester) ProcessUpgradeResponse(response *upgraderesponder.CheckUpgradeResponse, err error) {
	r.log.Info("Checking available version")
	if err != nil {
		r.log.Error(err, "failed to check upgrade")
		return
	}
	latestVersion := ""
	for _, v := range response.Versions {
		found := false
		for _, tag := range v.Tags {
			if tag == VersionTagLatest {
				found = true
				break
			}
		}
		if found {
			latestVersion = v.Name
			break
		}
	}
	if latestVersion == "" {
		r.log.Info("cannot find latest version in response", "response", response)
	} else {
		if semver.Compare(r.Version, latestVersion) < 0 {
			r.log.Info("Upgrade is available", "current-version", r.Version, "latest-version", latestVersion)
		} else {
			r.log.Info("Opni is the latest version")
		}
	}
}
