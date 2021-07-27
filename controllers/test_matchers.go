package controllers

import (
	"errors"

	gtypes "github.com/onsi/gomega/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ownershipMatcher struct {
	owner client.Object
}

func (o ownershipMatcher) Match(target interface{}) (success bool, err error) {
	if target, ok := target.(client.Object); ok {
		ownerRefs := target.GetOwnerReferences()
		if len(ownerRefs) == 0 {
			return false, nil
		}
		for _, ref := range ownerRefs {
			if ref.UID == o.owner.GetUID() &&
				ref.Kind == o.owner.GetObjectKind().GroupVersionKind().Kind &&
				ref.APIVersion == o.owner.GetObjectKind().GroupVersionKind().GroupVersion().String() &&
				ref.Name == o.owner.GetName() {
				return true, nil
			}
		}
	} else {
		return false, errors.New("target is not a client.Object")
	}
	return false, nil
}

func (o ownershipMatcher) FailureMessage(target interface{}) (message string) {
	return "expected " + target.(client.Object).GetName() + " to be owned by " + o.owner.GetName()
}

func (o ownershipMatcher) NegatedFailureMessage(target interface{}) (message string) {
	return "expected " + target.(client.Object).GetName() + " not to be owned by " + o.owner.GetName()
}

func IsOwnedBy(owner client.Object) gtypes.GomegaMatcher {
	return &ownershipMatcher{owner: owner}
}

var BeOwnedBy = IsOwnedBy
var OwnedBy = IsOwnedBy
