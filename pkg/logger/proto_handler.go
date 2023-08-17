package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type protoHandler struct {
	opts        slog.HandlerOptions
	level       slog.Leveler
	addSource   bool
	replaceAttr func([]string, slog.Attr) slog.Attr
	attrs       []*controlv1.Attr
	groups      []string // all groups started from WithGroup
	mu          sync.Mutex
	w           io.Writer
}

func NewProtoHandler(w io.Writer, opts *slog.HandlerOptions) *protoHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{
			Level:     DefaultLogLevel,
			AddSource: true,
		}
	}

	return &protoHandler{
		w:           w,
		level:       opts.Level,
		addSource:   opts.AddSource,
		opts:        *opts,
		replaceAttr: opts.ReplaceAttr,
	}
}

func (h *protoHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *protoHandler) clone() *protoHandler {
	return &protoHandler{
		attrs:       h.attrs,
		groups:      h.groups,
		w:           h.w,
		level:       h.level,
		addSource:   h.addSource,
		replaceAttr: h.replaceAttr,
	}
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

func (h *protoHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := h.clone()
	h2.groups = append(h2.groups, name)
	return h2
}

func (h *protoHandler) Handle(_ context.Context, r slog.Record) error {
	// get a buffer from the sync pool
	buf := newBuffer()
	defer buf.Free()

	replace := h.replaceAttr

	// write time
	var time *timestamppb.Timestamp
	if replace == nil {
		time = timestamppb.New(r.Time)
	} else if a := replace(nil /* groups */, slog.Time(slog.TimeKey, r.Time)); a.Key != "" {
		if t, ok := a.Value.Any().(*timestamppb.Timestamp); ok {
			time = t
		} else {
			return fmt.Errorf("invalid timestamp %v", a.Value)
		}
	}

	// write message
	var msg string
	if replace == nil {
		msg = r.Message
	} else if a := replace(nil /* groups */, slog.String(slog.MessageKey, r.Message)); a.Key != "" {
		msg = a.Value.String()
	}

	// write name
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
			source := &slog.Source{
				Function: f.Function,
				File:     f.File,
				Line:     f.Line,
			}

			if replace == nil {
				dir, file := filepath.Split(source.File)
				src.WriteString(filepath.Base(dir))
				src.WriteString("/")
				src.WriteString(file)
				src.WriteString(":")
				src.WriteString(strconv.Itoa(source.Line))
			} else if a := replace(nil /* groups */, slog.Any(slog.SourceKey, source)); a.Key != "" {
				src.WriteString(a.Value.String())
			}
		}
	}

	var attrs []*controlv1.Attr
	// write handler attributes
	if h.attrs != nil {
		attrs = h.attrs
	}

	// write attributes
	r.Attrs(func(attr slog.Attr) bool {
		attrs = h.appendAttr(attrs, attr)
		return true
	})

	structuredLogRecord := &controlv1.StructuredLogRecord{
		Time:       time,
		Message:    msg,
		Name:       name.String(),
		Source:     src.String(),
		Level:      r.Level.String(),
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

func (h *protoHandler) appendAttr(attrs []*controlv1.Attr, attr slog.Attr) []*controlv1.Attr {
	if attr.Equal(slog.Attr{}) {
		return attrs
	}

	attr.Value = attr.Value.Resolve()

	protoAttr := &controlv1.Attr{
		Key:   attr.Key,
		Value: attr.Value.String(),
	}

	return append(attrs, protoAttr)
}
