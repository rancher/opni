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

type OpniURN struct {
	Namespace string
	Type      UpdateType
	Strategy  string
	Component string
}

func ParseString(urn string) (OpniURN, error) {
	splitURN := strings.Split(urn, ":")

	if len(splitURN) != 5 {
		return OpniURN{}, ErrInvalidURN
	}

	if splitURN[0] != "urn" {
		return OpniURN{}, ErrInvalidURN
	}

	if splitURN[1] != Namespace {
		return OpniURN{}, ErrInvalidNamespace(splitURN[1])
	}

	return OpniURN{
		Namespace: splitURN[1],
		Type:      UpdateType(splitURN[2]),
		Strategy:  splitURN[3],
		Component: splitURN[4],
	}, nil
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
	return fmt.Sprintf("urn:%s:%s:%s:%s", Namespace, u.Type, u.Strategy, u.Component)
}

func ErrInvalidNamespace(ns string) error {
	return fmt.Errorf("invalid namespace - %s: %w", ns, ErrInvalidURN)
}
