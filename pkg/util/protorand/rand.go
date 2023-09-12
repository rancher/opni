package protorand

// Contains a generic wrapper around protorand.

import (
	"math"
	"math/rand"

	art "github.com/plar/go-adaptive-radix-tree"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/fieldmask"
	"github.com/sryoya/protorand"
	"github.com/zeebo/xxh3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protopath"
	"google.golang.org/protobuf/reflect/protorange"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type ProtoRand[T proto.Message] struct {
	*protorand.ProtoRand

	mask *fieldmaskpb.FieldMask
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

// If non-nil, fields listed in the mask will be unset in generated messages.
// When generating a partial message, listed fields will not be included
// in the list of possible fields to select from, as if the fields did not
// exist in the message at all. Paths in the mask are relative to the root
// of the message and can include nested fields. If nested fields are listed,
// the mask only applies to "leaf" fields, and not to containing messages.
func (p *ProtoRand[T]) ExcludeMask(mask *fieldmaskpb.FieldMask) {
	p.mask = mask
}

func (p *ProtoRand[T]) Gen() (T, error) {
	out, err := p.ProtoRand.Gen(util.NewMessage[T]())
	if err != nil {
		var zero T
		return zero, err
	}
	if p.mask != nil {
		fieldmask.ExclusiveDiscard(out, p.mask)
	}
	sanitizeLargeNumbers(out)
	return out.(T), nil
}

func (p *ProtoRand[T]) MustGen() T {
	out, err := p.Gen()
	if err != nil {
		panic(err)
	}
	return out
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

	var nestedMask art.Tree
	if p.mask != nil {
		nestedMask = fieldmask.AsTree(p.mask)
	}

	var walk func(msg protoreflect.Message, prefix string, mask art.Tree)
	walk = func(msg protoreflect.Message, prefix string, mask art.Tree) {
		md := msg.Descriptor()
		wire, _ := proto.MarshalOptions{Deterministic: true}.Marshal(msg.Interface())
		msgFields := md.Fields()
		selectedFields := make([]protoreflect.FieldDescriptor, 0, msgFields.Len())
		for i := 0; i < msgFields.Len(); i++ {
			msgField := msgFields.Get(i)
			included := true
			if mask != nil {
				if _, masked := mask.Search(art.Key(prefix + string(msgField.Name()))); masked {
					// only exclude leaf fields from the mask
					included = false
				}
			}
			if included {
				selectedFields = append(selectedFields, msgField)
			} else {
				msg.Clear(msgField)
			}
		}

		partition := newPartition(len(selectedFields), ratio)
		rand.New(rand.NewSource(int64(xxh3.Hash(wire)))).
			Shuffle(len(partition), func(i, j int) {
				partition[i], partition[j] = partition[j], partition[i]
			})
		for i, msgField := range selectedFields {
			if partition[i] == 0 {
				msg.Clear(msgField)
			} else if !msgField.IsList() && !msgField.IsMap() && msgField.Kind() == protoreflect.MessageKind {
				walk(msg.Mutable(msgField).Message(), prefix+string(msgField.Name())+".", mask)
			}
		}
	}
	walk(newGeneratedMsg.ProtoReflect(), "", nestedMask)
	return newGeneratedMsg, nil
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
	numOnes := max(1, int(math.Round(ratio*float64(size))))
	for i := 0; i < numOnes; i++ {
		s[i] = 1
	}
	return s
}

func sanitizeLargeNumbers(msg proto.Message) {
	protorange.Range(msg.ProtoReflect(), func(vs protopath.Values) error {
		// mask randomly generated uint64s to 53 bits, as larger values are not
		// representable in json.
		v := vs.Index(-1)
		if v.Step.Kind() != protopath.FieldAccessStep {
			return nil
		}
		fd := v.Step.FieldDescriptor()
		if fd.Kind() == protoreflect.Uint64Kind {
			u := v.Value.Uint()
			if masked := u & 0x1FFFFFFFFFFFFF; masked != u {
				containingMsg := vs.Index(-2).Value.Message()
				containingMsg.Set(fd, protoreflect.ValueOfUint64(masked))
			}
		}
		return nil
	})
}
