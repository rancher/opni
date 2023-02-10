package session

import (
	"context"
	"crypto/subtle"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	authutil "github.com/rancher/opni/pkg/auth/util"
	"github.com/rancher/opni/pkg/util/streams"
	"golang.org/x/crypto/blake2b"

	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/keyring"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type ServerChallenge struct {
	attrLoader
}

func NewServerChallenge(kr keyring.Keyring) (*ServerChallenge, error) {
	attr, err := loadAttributes(kr)
	if err != nil {
		return nil, err
	}

	return &ServerChallenge{
		attrLoader: attr,
	}, nil
}

func (a *ServerChallenge) InterceptContext(ctx context.Context) context.Context {
	return ctx
}

func (a *ServerChallenge) DoChallenge(ss streams.Stream) (context.Context, error) {
	challengeRequests := corev1.ChallengeRequestList{}
	var reqAttributes []Attribute

	id := cluster.StreamAuthorizedID(ss.Context())
	sharedKeys := cluster.StreamAuthorizedKeys(ss.Context())

	md, ok := metadata.FromIncomingContext(ss.Context())
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "metadata not found in context")
	}

	for _, v := range md.Get(AttributeMetadataKey) {
		if attr, ok := a.attributesByName[v]; ok {
			cr := &corev1.ChallengeRequest{
				Challenge: authutil.NewRandom256(),
			}
			challengeRequests.Items = append(challengeRequests.Items, cr)
			reqAttributes = append(reqAttributes, attr)
		} else {
			return nil, status.Errorf(codes.InvalidArgument, "unknown/unsupported session attribute: %s", v)
		}
	}

	if err := ss.SendMsg(&challengeRequests); err != nil {
		return nil, err
	}

	challengeResponses := corev1.ChallengeResponseList{}
	if err := ss.RecvMsg(&challengeResponses); err != nil {
		return nil, err
	}

	if len(challengeResponses.Items) != len(challengeRequests.Items) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid number of challenge responses")
	}

	var failed bool
	for i, challengeResponse := range challengeResponses.Items {
		challengeRequest := challengeRequests.Items[i]
		attr := reqAttributes[i]
		ok := attr.Verify(id, challengeRequest.Challenge, challengeResponse.Response)
		if !ok {
			failed = true
		}
	}

	if failed {
		return nil, status.Errorf(codes.Unauthenticated, "session attribute verification failed")
	}

	sessionInfo := &corev1.SessionInfo{}

	{
		// compute a mac over the id, challenge requests, and responses
		mac, _ := blake2b.New512(sharedKeys.ServerKey)
		mac.Write([]byte(id))
		for _, cr := range challengeRequests.Items {
			mac.Write(cr.Challenge)
		}
		for _, cr := range challengeResponses.Items {
			mac.Write(cr.Response)
		}
		for _, attr := range reqAttributes {
			mac.Write([]byte(attr.Name()))
			sessionInfo.Attributes = append(sessionInfo.Attributes, attr.Name())
		}
		sessionInfo.Mac = mac.Sum(nil)
	}

	if err := ss.SendMsg(sessionInfo); err != nil {
		return nil, err
	}

	ctx := context.WithValue(ss.Context(), AttributesContextKey, reqAttributes)

	return ctx, nil
}

type ClientChallenge struct {
	attrLoader
}

func NewClientChallenge(kr keyring.Keyring) (*ClientChallenge, error) {
	attr, err := loadAttributes(kr)
	if err != nil {
		return nil, err
	}

	return &ClientChallenge{
		attrLoader: attr,
	}, nil
}

func (a *ClientChallenge) InterceptContext(ctx context.Context) context.Context {
	if len(a.attributes) == 0 {
		return ctx
	}
	kvs := make([]string, 0, len(a.attributes)*2)
	for _, attr := range a.attributes {
		kvs = append(kvs, AttributeMetadataKey, attr.Name())
	}
	return metadata.AppendToOutgoingContext(ctx, kvs...)
}

func (a *ClientChallenge) DoChallenge(cs streams.Stream) (context.Context, error) {
	id := cluster.StreamAuthorizedID(cs.Context())
	sharedKeys := cluster.StreamAuthorizedKeys(cs.Context())

	var challengeRequests corev1.ChallengeRequestList
	if err := cs.RecvMsg(&challengeRequests); err != nil {
		return nil, err
	}
	if len(challengeRequests.Items) != len(a.attributes) {
		return nil, status.Errorf(codes.Internal, "server sent the wrong number of challenge requests")
	}

	challengeResponses := corev1.ChallengeResponseList{
		Items: make([]*corev1.ChallengeResponse, len(challengeRequests.Items)),
	}

	for i, challenge := range challengeRequests.Items {
		attr := a.attributes[i]
		mac := attr.Solve(id, challenge.Challenge)
		challengeResponses.Items[i] = &corev1.ChallengeResponse{
			Response: mac,
		}
	}

	if err := cs.SendMsg(&challengeResponses); err != nil {
		return nil, err
	}

	var sessionInfo corev1.SessionInfo
	if err := cs.RecvMsg(&sessionInfo); err != nil {
		return nil, err
	}

	// verify the mac
	{
		mac, _ := blake2b.New512(sharedKeys.ServerKey)
		mac.Write([]byte(id))
		for _, cr := range challengeRequests.Items {
			mac.Write(cr.Challenge)
		}
		for _, cr := range challengeResponses.Items {
			mac.Write(cr.Response)
		}
		for _, attr := range a.attributes {
			mac.Write([]byte(attr.Name()))
		}
		expectedMac := mac.Sum(nil)
		if subtle.ConstantTimeCompare(expectedMac, sessionInfo.Mac) != 1 {
			return nil, status.Errorf(codes.Aborted, "session info verification failed")
		}
	}

	ctx := context.WithValue(cs.Context(), AttributesContextKey, a.attributes)
	return ctx, nil
}

type attrLoader struct {
	attributes       []Attribute
	attributesByName map[string]Attribute
}

func (a *attrLoader) Attributes() []string {
	var names []string
	for _, attr := range a.attributes {
		names = append(names, attr.Name())
	}
	return names
}

// Matches challenges.ConditionFunc
func (a *attrLoader) HasAttributes(_ context.Context) (bool, error) {
	return len(a.attributes) > 0, nil
}

func loadAttributes(kr keyring.Keyring) (_ attrLoader, err error) {
	var attrs []Attribute
	kr.Try(func(ek *keyring.EphemeralKey) {
		if err != nil {
			return
		}
		if v, ok := ek.Labels[AttributeLabelKey]; ok {
			var attr Attribute
			attr, err = NewAttribute(v, ek.Secret)
			if err != nil {
				return
			}
			attrs = append(attrs, attr)
		}
	})
	if err != nil {
		return
	}

	attributesByName := map[string]Attribute{}
	for _, attr := range attrs {
		attributesByName[attr.Name()] = attr
	}

	return attrLoader{
		attributes:       attrs,
		attributesByName: attributesByName,
	}, nil
}

// Checks if the incoming context has the session attribute metadata key.
// Can be used in conjunction with a conditional challenge handler on the
// server side.
func ShouldEnableIncoming(streamContext context.Context) (bool, error) {
	if md, ok := metadata.FromIncomingContext(streamContext); ok {
		if v := md.Get(AttributeMetadataKey); len(v) > 0 {
			return true, nil
		}
	}
	return false, nil
}
