package protorand

// Contains a generic wrapper around protorand.

import (
	"math"
	"math/rand"

	"github.com/rancher/opni/pkg/util"
	"github.com/sryoya/protorand"
	"github.com/zeebo/xxh3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protopath"
	"google.golang.org/protobuf/reflect/protorange"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type ProtoRand[T proto.Message] struct {
	*protorand.ProtoRand

	seed int64
}

func New[T proto.Message]() *ProtoRand[T] {
	return &ProtoRand[T]{
		ProtoRand: protorand.New(),
	}
}

func (p *ProtoRand[T]) Seed(seed int64) {
	p.seed = seed
	p.ProtoRand.Seed(seed)
}

func (p *ProtoRand[T]) Gen() (T, error) {
	out, err := p.ProtoRand.Gen(util.NewMessage[T]())
	if err != nil {
		var zero T
		return zero, err
	}
	return out.(T), nil
}

func (p *ProtoRand[T]) MustGen() T {
	out, err := p.ProtoRand.Gen(util.NewMessage[T]())
	if err != nil {
		panic(err)
	}
	return out.(T)
}

// Generate a message with a specific ratio of set/unset fields.
//
// If ratio is 0, the message will be empty. If it is 1, all fields will be set.
// Otherwise, a randomly chosen subset of fields will be set. The ratio of set
// fields to unset fields is given by min(1, round(ratio * size)) where size
// is the number of fields in the current message. This function applies
// recursively to all fields, using the original ratio.
//
// This function reads the same amount of randomness from the underlying
// random number generator as a single call to Gen().
func (p *ProtoRand[T]) GenPartial(ratio float64) (T, error) {
	if ratio <= 0 {
		return util.NewMessage[T](), nil
	} else if ratio >= 1 {
		return p.Gen()
	}

	newGeneratedMsg := p.MustGen()

	options := protorange.Options{
		Stable: true,
	}
	return newGeneratedMsg, options.Range(newGeneratedMsg.ProtoReflect(), func(vs protopath.Values) error {
		v := vs.Index(0)
		isMsg := false
		switch v.Step.Kind() {
		case protopath.RootStep:
			isMsg = true
		case protopath.FieldAccessStep:
			isMsg = v.Step.FieldDescriptor().Kind() == protoreflect.MessageKind
		default:
			return nil
		}
		if !isMsg {
			return nil
		}
		msg := v.Value.Message()
		wire, _ := proto.MarshalOptions{Deterministic: true}.Marshal(msg.Interface())
		msgFields := msg.Descriptor().Fields()
		partition := newPartition(msgFields.Len(), ratio)
		rand.New(rand.NewSource(int64(xxh3.Hash(wire)))).
			Shuffle(len(partition), func(i, j int) {
				partition[i], partition[j] = partition[j], partition[i]
			})
		for i := 0; i < msgFields.Len(); i++ {
			if partition[i] == 0 {
				msg.Clear(msgFields.Get(i))
			}
		}
		return nil
	}, nil)
}

func (p *ProtoRand[T]) MustGenPartial(ratio float64) T {
	out, err := p.GenPartial(ratio)
	if err != nil {
		panic(err)
	}
	return out
}

// Returns a slice of a given size containing min(1, round(ratio * size)) 1s
// followed by 0s for the remaining elements.
func newPartition(size int, ratio float64) []int {
	if size == 0 {
		return nil
	}
	s := make([]int, size)
	numOnes := min(1, int(math.Round(ratio*float64(size))))
	for i := 0; i < numOnes; i++ {
		s[i] = 1
	}
	return s
}
