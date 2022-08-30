package cortex

import (
	"errors"
	"runtime/debug"
	"sync"

	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	"k8s.io/client-go/util/retry"
)

var (
	cortexVersion         string
	loadCortexVersionOnce sync.Once
)

func (r *Reconciler) updateCortexVersionStatus() (bool, error) {
	lg := r.logger
	loadCortexVersionOnce.Do(func() {
		buildInfo, ok := debug.ReadBuildInfo()
		if !ok {
			panic("could not read build info")
		}
		// https://github.com/golang/go/issues/33976
		if buildInfo.Main.Path == "" {
			cortexVersion = "(unknown)"
			return
		}
		for _, depInfo := range buildInfo.Deps {
			if depInfo.Path == "github.com/cortexproject/cortex" {
				if depInfo.Replace != nil {
					cortexVersion = depInfo.Replace.Version
				} else {
					cortexVersion = depInfo.Version
				}
				return
			}
		}
		panic("could not find cortex dependency in build info")
	})

	switch cluster := r.mc.(type) {
	case *v1beta2.MonitoringCluster:
		if cluster.Status.Cortex.Version != cortexVersion {
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				cluster.Status.Cortex.Version = cortexVersion
				return r.client.Status().Update(r.ctx, cluster)
			})
			if err != nil {
				lg.Error(err, "failed to update cortex version status")
				return false, err
			}
			return true, nil
		}
	case *corev1beta1.MonitoringCluster:
		if cluster.Status.Cortex.Version != cortexVersion {
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				cluster.Status.Cortex.Version = cortexVersion
				return r.client.Status().Update(r.ctx, cluster)
			})
			if err != nil {
				lg.Error(err, "failed to update cortex version status")
				return false, err
			}
			return true, nil
		}
	default:
		return false, errors.New("unsupported monitoring type")
	}

	return false, nil
}
