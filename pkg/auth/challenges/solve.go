package challenges

import (
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"golang.org/x/crypto/blake2b"
)

func Solve(req *corev1.ChallengeRequest, cm ClientMetadata, key []byte, domain string) *corev1.ChallengeResponse {
	kh, err := blake2b.New512(key[:])
	if err != nil {
		panic(err)
	}
	kh.Write([]byte(domain))
	kh.Write([]byte(cm.IdAssertion))
	kh.Write(cm.Random)
	kh.Write(req.Challenge)
	return &corev1.ChallengeResponse{
		Response: kh.Sum(nil),
	}
}
