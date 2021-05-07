package deploy

import (
	"context"
	"time"

	"github.com/rancher/wrangler/pkg/objectset"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func Delete(ctx context.Context, sc *Context, deleteAll bool) error {
	var skipOpni, skipInfra, skipServices bool
	servicesOwner, err := getOwner(ctx, ServicesStack, sc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			skipServices = true
			logrus.Info("services stack already deleted")
		} else {
			return err
		}

	}
	if !skipServices {
		// deleting Services stack
		logrus.Infof("Deleting Services stack")
		opniObjs, _, err := objs(ServicesStack, nil)
		if err != nil {
			return err
		}
		os := objectset.NewObjectSet()
		os.Add(opniObjs...)
		var gvk []schema.GroupVersionKind
		for k := range os.ObjectsByGVK() {
			gvk = append(gvk, k)
		}
		if err := sc.Apply.WithOwner(servicesOwner).WithGVK(gvk...).WithSetID(ServicesStack).Apply(nil); err != nil {
			return err
		}
	}

	opniOwner, err := getOwner(ctx, OpniStack, sc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			skipOpni = true
			logrus.Info("Opni stack already deleted")
		} else {
			return err
		}

	}
	if !skipOpni {
		// deleting opni stack
		logrus.Infof("Deleting opni stack")
		opniObjs, _, err := objs(OpniStack, nil)
		if err != nil {
			return err
		}
		os := objectset.NewObjectSet()
		os.Add(opniObjs...)
		var gvk []schema.GroupVersionKind
		for k := range os.ObjectsByGVK() {
			gvk = append(gvk, k)
		}
		if err := sc.Apply.WithOwner(opniOwner).WithGVK(gvk...).WithSetID(OpniStack).Apply(nil); err != nil {
			return err
		}
	}
	// deleting infra stack if --all is passed
	if deleteAll {
		if !skipOpni {
			time.Sleep(1 * time.Minute)
		}
		infraOwner, err := getOwner(ctx, InfraStack, sc)
		if err != nil {
			if apierrors.IsNotFound(err) {
				skipInfra = true
				logrus.Info("Infra stack already deleted")
			} else {
				return err
			}
		}
		if !skipInfra {
			infraObjs, _, err := objs(InfraStack, nil)
			if err != nil {
				return err
			}
			os := objectset.NewObjectSet()
			os.Add(infraObjs...)
			var gvk []schema.GroupVersionKind
			for k := range os.ObjectsByGVK() {
				gvk = append(gvk, k)
			}
			logrus.Infof("Deleting infra resources")
			if err := sc.Apply.WithOwner(infraOwner).WithGVK(gvk...).WithSetID(InfraStack).Apply(nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func getOwner(ctx context.Context, name string, sc *Context) (*corev1.ConfigMap, error) {
	return sc.K8s.CoreV1().ConfigMaps("kube-system").Get(ctx, name, metav1.GetOptions{})
}
