package core

type Comparator[T any] interface {
	Equal(other T) bool
}

func (c *TokenCapability) Equal(other *TokenCapability) bool {
	return equalNonzeroOperator(c.GetType(), other.GetType()) &&
		equalNonzeroFunc(c.GetReference(), other.GetReference())
}

func (c *ClusterCapability) Equal(other *ClusterCapability) bool {
	return equalNonzeroOperator(c.GetName(), other.GetName())
}

func (r *Reference) Equal(other *Reference) bool {
	if r == nil || other == nil {
		return r == other
	}
	return equalNonzeroOperator(r.Id, other.Id)
}

func equalNonzeroFunc[T Comparator[T]](a, b T) bool {
	var zero T
	if a.Equal(zero) || b.Equal(zero) {
		return false
	}
	return a.Equal(b)
}

func equalNonzeroOperator[T comparable](a, b T) bool {
	var zero T
	if a == zero || b == zero {
		return false
	}
	return a == b
}
