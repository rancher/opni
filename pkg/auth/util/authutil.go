package authutil

import (
	"crypto/rand"
	"fmt"

	"log/slog"

	"github.com/rancher/opni/pkg/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func CheckUnknownFields(lg *slog.Logger, msg proto.Message) error {
	if len(msg.ProtoReflect().GetUnknown()) > 0 {
		err := status.Errorf(codes.InvalidArgument, "expected challenge response, but received incorrect message data (agent possibly incompatible or misconfigured)")
		lg.With(
			zap.Error(err),
			"unknownFields", fmt.Sprintf("[redacted (len: %d)]", len(msg.ProtoReflect().GetUnknown())),
		).Debug("agent failed to authenticate")
		return err
	}
	return nil
}

func NewRandom256() []byte {
	var challenge [32]byte
	_, err := rand.Read(challenge[:])
	if err != nil {
		panic(err)
	}
	return challenge[:]
}
