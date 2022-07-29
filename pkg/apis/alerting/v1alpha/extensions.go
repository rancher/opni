package v1alpha

/*
TODO
Meaningful defaults for endpoint implementations
*/

func (s *SlackImplementation) Defaults() *SlackImplementation {
	return &SlackImplementation{}
}

func (e *EmailImplementation) Defaults() *EmailImplementation {
	return &EmailImplementation{}
}
