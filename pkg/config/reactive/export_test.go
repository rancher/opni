package reactive

import (
	"fmt"
	"io"

	art "github.com/plar/go-adaptive-radix-tree"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func (s *Controller[T]) DebugDumpReactiveMessagesInfo(out io.Writer) {
	s.reactiveMessagesMu.Lock()
	defer s.reactiveMessagesMu.Unlock()

	s.reactiveMessages.ForEach(func(node art.Node) (cont bool) {
		switch node.Kind() {
		case art.Leaf:
			rm := node.Value().(*reactiveValue)
			numWatchers := 0
			rm.watchChannels.Range(func(_ string, _ chan protoreflect.Value) bool {
				numWatchers++
				return true
			})
			rm.watchFuncs.Range(func(_ string, _ *func(int64, protoreflect.Value)) bool {
				numWatchers++
				return true
			})
			fmt.Fprintf(out, "message %s: {watchers: %d; rev: %d}\n", node.Key(), numWatchers, rm.rev)
		default:
			fmt.Fprintf(out, "node: key: %s; value: %v\n", node.Key(), node.Value())
		}
		return true
	})
}

type ReactiveValue = reactiveValue
