package v1alpha1

import (
	thanosv1alpha1 "github.com/banzaicloud/thanos-operator/pkg/sdk/api/v1alpha1"
)

func init() {
	SchemeBuilder.Register(
		&thanosv1alpha1.Thanos{}, &thanosv1alpha1.ThanosList{},
		&thanosv1alpha1.ThanosPeer{}, &thanosv1alpha1.ThanosPeerList{},
		&thanosv1alpha1.ThanosEndpoint{}, &thanosv1alpha1.ThanosEndpointList{},
		&thanosv1alpha1.ThanosPeer{}, &thanosv1alpha1.ThanosPeerList{},
		&thanosv1alpha1.ObjectStore{}, &thanosv1alpha1.ObjectStoreList{},
		&thanosv1alpha1.Receiver{}, &thanosv1alpha1.ReceiverList{},
		&thanosv1alpha1.StoreEndpoint{}, &thanosv1alpha1.StoreEndpointList{},
	)
}
