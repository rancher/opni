package v1alpha

/*
TODO
Meaningful defaults for endpoint implementations
*/

func (s *SlackImplementation) Defaults() *SlackImplementation {
	return &SlackImplementation{
		Title:    "Your optional title here",
		Text:     "Your optional text here",
		Footer:   "Your optional footer here",
		ImageUrl: "Your optional image url here",
	}
}

func (e *EmailImplementation) Defaults() *EmailImplementation {
	hbody := "Your optional body here"
	tbody := "Your optional body here"
	return &EmailImplementation{
		HtmlBody: &hbody,
		TextBody: &tbody,
	}
}
