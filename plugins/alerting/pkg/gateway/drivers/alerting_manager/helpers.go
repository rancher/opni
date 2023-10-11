package alerting_manager

import (
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newAlertmanagerSet(ns string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shared.AlertmanagerService,
			Namespace: ns,
		},
	}
}

type statusTuple lo.Tuple2[error, *appsv1.StatefulSet]
