package logger

import (
	"context"
	"encoding"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// protoHandler outputs a size-prefixed []byte for every log message

type protoHandler struct {
	level       slog.Leveler
	addSource   bool
	replaceAttr func([]string, slog.Attr) slog.Attr
	attrs       []*controlv1.Attr // attrs started from With
	groups      []string          // all groups started from WithGroup
	groupPrefix string            // groups started from Group
	mu          sync.Mutex
	w           io.Writer
}

func newProtoHandler(w io.Writer, opts *slog.HandlerOptions) *protoHandler {
	if opts.Level == nil {
		opts.Level = DefaultLogLevel
	}

	return &protoHandler{
		w:           w,
		level:       opts.Level,
		addSource:   opts.AddSource,
		replaceAttr: opts.ReplaceAttr,
	}
}

func (h *protoHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *protoHandler) clone() *protoHandler {
	return &protoHandler{
		level:       h.level,
		addSource:   h.addSource,
		replaceAttr: h.replaceAttr,
		attrs:       h.attrs,
		groups:      h.groups,
		groupPrefix: h.groupPrefix,
		w:           h.w,
	}
}

func (h *protoHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := h.clone()
	h2.groupPrefix += name + "."
	h2.groups = append(h2.groups, name)
	return h2
}

func (h *protoHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	h2 := h.clone()

	for _, attr := range attrs {
		if h.replaceAttr != nil {
			attr = h.replaceAttr(h.groups, attr)
		}
		h2.attrs = h2.appendAttr(h.attrs, attr)
	}
	return h2
}

func (h *protoHandler) Handle(_ context.Context, r slog.Record) error {
	// get a buffer from the sync pool
	buf := newBuffer()
	defer buf.Free()

	replace := h.replaceAttr

	// write time
	var timestamp *timestamppb.Timestamp
	if replace == nil {
		timestamp = timestamppb.New(r.Time)
	} else if a := replace(nil /* groups */, slog.Time(slog.TimeKey, r.Time)); a.Key != "" {
		if t, ok := a.Value.Any().(*timestamppb.Timestamp); ok {
			timestamp = t
		} else if t, ok := a.Value.Any().(time.Time); ok {
			timestamp = timestamppb.New(t)
		} else {
			timestamp = timestamppb.Now() // overwrites invalid timestamps created from replaceAttr
		}
	}

	// write level
	var lvl string
	if replace == nil {
		lvl = r.Level.String()
	} else if a := replace(nil /* groups */, slog.Any(slog.LevelKey, r.Level)); a.Key != "" {
		lvl = h.toString(a.Value)
	}

	// write logger name
	var name strings.Builder
	last := len(h.groups) - 1
	for i, group := range h.groups {
		name.WriteString(group)
		if i < last {
			name.WriteString(".")
		}
	}

	// write source
	var src strings.Builder
	if h.addSource {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		if f.File != "" {
			if h.replaceAttr == nil {
				h.appendSource(&src, f.File, f.Line)
			} else if a := h.replaceAttr(nil /* groups */, slog.Any(slog.SourceKey, &slog.Source{
				Function: f.Function,
				File:     f.File,
				Line:     f.Line,
			})); a.Key != "" {
				src.WriteString(h.toString(a.Value))
			}
		}
	}

	// write message
	var msg string
	if replace == nil {
		msg = r.Message
	} else if a := replace(nil /* groups */, slog.String(slog.MessageKey, r.Message)); a.Key != "" {
		msg = a.Value.String()
	}

	var attrs []*controlv1.Attr
	// write handler attributes
	if h.attrs != nil {
		attrs = h.attrs
	}

	// write attributes
	r.Attrs(func(attr slog.Attr) bool {
		if h.replaceAttr != nil {
			attr = h.replaceAttr(h.groups, attr)
		}
		attrs = h.appendAttr(attrs, attr)
		return true
	})

	structuredLogRecord := &controlv1.StructuredLogRecord{
		Time:       timestamp,
		Message:    msg,
		Name:       name.String(),
		Source:     src.String(),
		Level:      lvl,
		Attributes: attrs,
	}

	bytes, err := proto.Marshal(structuredLogRecord)
	if err != nil {
		return err
	}

	// prefix each record with its size
	size := len(bytes)
	sizeBuf := make([]byte, 4)
	sizeBuf[0] = byte(size)
	sizeBuf[1] = byte(size >> 8)
	sizeBuf[2] = byte(size >> 16)
	sizeBuf[3] = byte(size >> 24)

	// write the record
	buf.Write(sizeBuf)
	buf.Write(bytes)

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err = h.w.Write(*buf)
	return err
}

func (h *protoHandler) appendSource(src *strings.Builder, file string, line int) {
	dir, file := filepath.Split(file)

	src.WriteString(filepath.Base(dir))
	src.WriteByte('/')
	src.WriteString(file)
	src.WriteByte(':')
	src.WriteString(strconv.Itoa(line))
}

func (h *protoHandler) appendAttr(attrs []*controlv1.Attr, attr slog.Attr) []*controlv1.Attr {
	if attr.Equal(slog.Attr{}) {
		return attrs
	}

	attr.Value = attr.Value.Resolve()

	if attr.Value.Kind() == slog.KindGroup {
		for _, groupAttr := range attr.Value.Group() {
			attrs = h.appendAttr(attrs, groupAttr)
		}
		return attrs
	}

	protoAttr := &controlv1.Attr{
		Key:   attr.Key,
		Value: h.toString(attr.Value),
	}

	return append(attrs, protoAttr)
}

func (h *protoHandler) toString(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindInt64:
		return strconv.FormatInt(v.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(v.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(v.Float64(), 'g', -1, 64)
	case slog.KindBool:
		return fmt.Sprintf("%v", v.Bool())
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().String()
	case slog.KindAny:
		switch cv := v.Any().(type) {
		case slog.Level:
			return cv.Level().String()
		case encoding.TextMarshaler:
			data, err := cv.MarshalText()
			if err != nil {
				break
			}
			return string(data)
		case *slog.Source:
			var src strings.Builder
			h.appendSource(&src, cv.File, cv.Line)
			return src.String()
		default:
			return fmt.Sprint(v.Any())
		}
	}
	return "BADVALUE"
}
