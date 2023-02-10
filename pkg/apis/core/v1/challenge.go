package v1

import (
	"crypto/subtle"

	"golang.org/x/crypto/blake2b"
)

type ClientMetadata = struct {
	IdAssertion string
	Random      []byte
}

func (req *ChallengeRequest) Solve(cm ClientMetadata, key []byte) *ChallengeResponse {
	var buf [blake2b.Size]byte
	k := buf[:0]
	k = append(k, key...)
	copy(buf[:], key)
	kh, _ := blake2b.New512(k)
	kh.Write([]byte(cm.IdAssertion))
	kh.Write(cm.Random)
	kh.Write(req.Challenge)
	return &ChallengeResponse{
		Response: kh.Sum(nil),
	}
}

func (resp *ChallengeResponse) Equal(other *ChallengeResponse) int {
	if resp == other {
		panic("bug: comparing challenge response to itself")
	}
	return subtle.ConstantTimeCompare(resp.Response, other.Response)
}
