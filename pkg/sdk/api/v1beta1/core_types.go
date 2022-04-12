package v1beta1

import (
	"github.com/rancher/opni-monitoring/pkg/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//+kubebuilder:object:root=true
type BootstrapToken struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              *core.BootstrapToken `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true
type BootstrapTokenList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BootstrapToken `json:"items"`
}

//+kubebuilder:object:root=true
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              *core.Cluster `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cluster `json:"items"`
}

//+kubebuilder:object:root=true
type Role struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              *core.Role `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true
type RoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Role `json:"items"`
}

//+kubebuilder:object:root=true
type RoleBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              *core.RoleBinding `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true
type RoleBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RoleBinding `json:"items"`
}

//+kubebuilder:object:root=true
type Keyring struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Data              []byte `json:"data,omitempty"`
}

//+kubebuilder:object:root=true
type KeyringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Keyring `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&BootstrapToken{}, &BootstrapTokenList{},
		&Cluster{}, &ClusterList{},
		&Role{}, &RoleList{},
		&RoleBinding{}, &RoleBindingList{},
		&Keyring{}, &KeyringList{},
	)
}
