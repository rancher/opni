package urn

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidURN = errors.New("invalid URN")

const Namespace = "opni"

type UpdateType string

const (
	Plugin UpdateType = "plugin"
	Agent  UpdateType = "agent"
)

func AllUpdateTypes() []UpdateType {
	return []UpdateType{Plugin, Agent}
}

type OpniURN struct {
	Namespace string
	Type      UpdateType
	Strategy  string
	Component string
}

func ParseString(urn string) (OpniURN, error) {
	splitURN := strings.Split(urn, ":")
	if len(splitURN) != 5 {
		return OpniURN{}, fmt.Errorf("%w: incorrect number of fields", ErrInvalidURN)
	}

	u := OpniURN{
		Namespace: splitURN[1],
		Type:      UpdateType(splitURN[2]),
		Strategy:  splitURN[3],
		Component: splitURN[4],
	}
	if err := u.Validate(); err != nil {
		return OpniURN{}, err
	}
	return u, nil
}

func (u OpniURN) Validate() error {
	if u.Namespace == "" {
		return fmt.Errorf("%w: missing namespace", ErrInvalidURN)
	}
	if u.Namespace != Namespace {
		return fmt.Errorf("%w: invalid namespace: %s", ErrInvalidURN, u.Namespace)
	}
	if u.Type == "" {
		return fmt.Errorf("%w: missing type", ErrInvalidURN)
	}
	if u.Strategy == "" {
		return fmt.Errorf("%w: missing strategy", ErrInvalidURN)
	}
	if u.Component == "" {
		return fmt.Errorf("%w: missing component", ErrInvalidURN)
	}
	return nil
}

func NewOpniURN(updateType UpdateType, strategy, component string) OpniURN {
	return OpniURN{
		Namespace: Namespace,
		Type:      updateType,
		Strategy:  strategy,
		Component: component,
	}
}

func (u OpniURN) String() string {
	return fmt.Sprintf("urn:%s:%s:%s:%s", u.Namespace, u.Type, u.Strategy, u.Component)
}
