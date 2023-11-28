package k8sutil

import (
	"bytes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/structured-merge-diff/v4/fieldpath"
)

// Returns the field manager name for the given path, or "" if no manager is
// associated with the path.
func FieldManagerForPath(obj client.Object, path fieldpath.Path) string {
	for _, entry := range obj.GetManagedFields() {
		if DecodeManagedFieldsEntry(entry).Has(path) {
			return entry.Manager
		}
	}
	return ""
}

func DecodeManagedFieldsEntry(e metav1.ManagedFieldsEntry) *fieldpath.Set {
	var set fieldpath.Set
	if e.FieldsV1 != nil {
		set.FromJSON(bytes.NewReader(e.FieldsV1.Raw))
	}
	return &set
}
