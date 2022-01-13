package ident

import "context"

type fakeProvider struct {
	id string
}

func NewFakeProvider(id string) Provider {
	return &fakeProvider{
		id: id,
	}
}

func (p *fakeProvider) UniqueIdentifier(context.Context) (string, error) {
	return p.id, nil
}
