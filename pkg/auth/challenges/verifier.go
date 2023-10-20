package challenges

import (
	"context"
	"crypto/subtle"

	"log/slog"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"google.golang.org/grpc/codes"
)

type KeyringVerifier interface {
	Prepare(ctx context.Context, args ClientMetadata, req *corev1.ChallengeRequest) (PreCachedVerifier, error)
}

type PreCachedVerifier interface {
	Verify(response *corev1.ChallengeResponse) *keyring.SharedKeys
}

type keyringVerifier struct {
	domain             string
	keyringStoreBroker storage.KeyringStoreBroker
	logger             *slog.Logger
}

func NewKeyringVerifier(ksb storage.KeyringStoreBroker, domain string, lg *slog.Logger) KeyringVerifier {
	return &keyringVerifier{
		domain:             domain,
		keyringStoreBroker: ksb,
		logger:             lg,
	}
}

type preCachedSolution struct {
	keys     *keyring.SharedKeys
	solution *corev1.ChallengeResponse
}

func (v *keyringVerifier) Prepare(ctx context.Context, cm ClientMetadata, req *corev1.ChallengeRequest) (PreCachedVerifier, error) {
	ks := v.keyringStoreBroker.KeyringStore("gateway", &corev1.Reference{Id: cm.IdAssertion})
	kr, err := ks.Get(ctx)
	if err != nil {
		if storage.IsNotFound(err) {
			return nil, util.StatusError(codes.Unauthenticated)
		}
		v.logger.With(logger.Err(err)).Error("failed to get keyring during cluster auth")
		return nil, util.StatusError(codes.Unavailable)
	}

	possibleSolutions := []preCachedSolution{
		{}, // nil first element
	}

	kr.Try(func(shared *keyring.SharedKeys) {
		possibleSolutions = append(possibleSolutions, preCachedSolution{
			keys:     shared,
			solution: Solve(req, cm, shared.ClientKey, v.domain),
		})
	})
	return &preCachedVerifier{
		clientMetadata:    cm,
		possibleSolutions: possibleSolutions,
	}, nil
}

type preCachedVerifier struct {
	clientMetadata    ClientMetadata
	possibleSolutions []preCachedSolution
}

//go:noinline
//go:nosplit
func (v *preCachedVerifier) Verify(resp *corev1.ChallengeResponse) *keyring.SharedKeys {
	var verified int
	for i, l := 1, len(v.possibleSolutions); i < l; i++ {
		equal := subtle.ConstantTimeCompare(v.possibleSolutions[i].solution.Response, resp.Response)
		// set verified to i if equal==1 and verified==0
		verified = subtle.ConstantTimeSelect(subtle.ConstantTimeEq(int32(verified), 0), i*equal, verified)
	}
	return v.possibleSolutions[verified].keys
}
