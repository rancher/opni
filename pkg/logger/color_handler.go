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
	"sync"
	"time"
	"unicode"
)

// modified from https://github.com/lmittmann/tint
// adds custom colors and additional log fields
// note: colorHandler's behaviour differs from a typical slog handler such that
// 1. attribute keys are never prefixed with group names. key-val pairs created using Group() are logged without the group name
// 2. group names from WithGroup are always logged before the message
// 3. error keys are printed as errKey

const (
	ansiReset           = "\033[0m"
	ansiFaintStyle      = "\033[2m"
	ansiResetFaintStyle = "\033[22m"
	ansiBrightRed       = "\033[91m"
	ansiBrightGreen     = "\033[92m"
	ansiBrightYellow    = "\033[93m"
	ansiBrightBlue      = "\033[94m"
	ansiBrightMagenta   = "\033[95m"
	ansiBrightCyan      = "\033[96m"
	ansiBrightRedFaint  = "\033[91;2m"
)

type colorHandler struct {
	level        slog.Leveler
	addSource    bool
	replaceAttr  func([]string, slog.Attr) slog.Attr
	colorEnabled bool
	timeFormat   string
	attrsPrefix  string   // attrs started from With
	groups       []string // all groups started from WithGroup
	groupPrefix  string   // groups started from Group
	appendName   bool
	mu           sync.Mutex
	w            io.Writer
}

func newColorHandler(w io.Writer, opts *LoggerOptions) slog.Handler {
	if opts == nil {
		opts = &LoggerOptions{
			Level:        DefaultLogLevel,
			AddSource:    true,
			ColorEnabled: ColorEnabled(),
			AppendName:   true,
			TimeFormat:   DefaultTimeFormat,
		}
	}

	if opts.TimeFormat == "" {
		opts.TimeFormat = DefaultTimeFormat
	}

	return &colorHandler{
		level:        opts.Level,
		addSource:    opts.AddSource,
		replaceAttr:  opts.ReplaceAttr,
		colorEnabled: opts.ColorEnabled,
		timeFormat:   opts.TimeFormat,
		appendName:   opts.AppendName,
		w:            w,
	}
}

func (h *colorHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *colorHandler) clone() *colorHandler {
	return &colorHandler{
		level:        h.level,
		addSource:    h.addSource,
		replaceAttr:  h.replaceAttr,
		colorEnabled: h.colorEnabled,
		timeFormat:   h.timeFormat,
		attrsPrefix:  h.attrsPrefix,
		groups:       h.groups,
		groupPrefix:  h.groupPrefix,
		w:            h.w,
	}
}

func (h *colorHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := h.clone()
	h2.groupPrefix += name + "."
	h2.groups = append(h2.groups, name)
	return h2
}

func (h *colorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	h2 := h.clone()

	buf := newBuffer()
	defer buf.Free()

	for _, attr := range attrs {
		if h.replaceAttr != nil {
			attr = h.replaceAttr(h.groups, attr)
		}
		h.appendAttr(buf, attr, h.groupPrefix)
	}
	h2.attrsPrefix = h.attrsPrefix + string(*buf)
	return h2
}

func (h *colorHandler) Handle(_ context.Context, r slog.Record) error {
	buf := newBuffer()
	defer buf.Free()

	rep := h.replaceAttr

	h.appendTime(buf, r.Time)

	if rep == nil {
		h.appendLevel(buf, r.Level)
		buf.WriteByte(' ')
	} else if a := rep(nil /* groups */, slog.Any(slog.LevelKey, r.Level)); a.Key != "" {
		h.appendValue(buf, a.Value, false)
		buf.WriteByte(' ')
	}

	if h.appendName {
		h.writeGroups(buf)
	}

	if h.addSource {
		h.writeSource(buf, r.PC)
	}

	h.writeMessage(buf, r.Message)

	// write handler attributes
	if len(h.attrsPrefix) > 0 {
		buf.WriteString(h.attrsPrefix)
	}

	// write attributes
	r.Attrs(func(attr slog.Attr) bool {
		if rep != nil {
			attr = rep(h.groups, attr)
		}
		h.appendAttr(buf, attr, h.groupPrefix)
		return true
	})

	if len(*buf) == 0 {
		return nil
	}
	(*buf)[len(*buf)-1] = '\n' // replace last space with newline

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(*buf)
	return err
}

func (h *colorHandler) appendTime(buf *buffer, t time.Time) {
	buf.WriteStringIf(h.colorEnabled, ansiFaintStyle)
	if h.replaceAttr == nil {
		*buf = t.AppendFormat(*buf, h.timeFormat)
		buf.WriteByte(' ')
	} else if a := h.replaceAttr(nil /* groups */, slog.Time(slog.TimeKey, t)); a.Key != "" {
		h.appendValue(buf, a.Value, false)
		buf.WriteByte(' ')
	}
	buf.WriteStringIf(h.colorEnabled, ansiReset)
}

func (h *colorHandler) appendLevel(buf *buffer, level slog.Level) {
	switch {
	case level < slog.LevelInfo:
		buf.WriteStringIf(h.colorEnabled, ansiBrightMagenta)
		buf.WriteString("DEBUG")
		appendLevelDelta(buf, level-slog.LevelDebug)
		buf.WriteStringIf(h.colorEnabled, ansiReset)
	case level < slog.LevelWarn:
		buf.WriteStringIf(h.colorEnabled, ansiBrightBlue)
		buf.WriteString("INFO")
		appendLevelDelta(buf, level-slog.LevelInfo)
		buf.WriteStringIf(h.colorEnabled, ansiReset)
	case level < slog.LevelError:
		buf.WriteStringIf(h.colorEnabled, ansiBrightYellow)
		buf.WriteString("WARN")
		appendLevelDelta(buf, level-slog.LevelWarn)
		buf.WriteStringIf(h.colorEnabled, ansiReset)
	default:
		buf.WriteStringIf(h.colorEnabled, ansiBrightRed)
		buf.WriteString("ERROR")
		appendLevelDelta(buf, level-slog.LevelError)
		buf.WriteStringIf(h.colorEnabled, ansiReset)
	}

}

func appendLevelDelta(buf *buffer, delta slog.Level) {
	if delta == 0 {
		return
	} else if delta > 0 {
		buf.WriteByte('+')
	}
	*buf = strconv.AppendInt(*buf, int64(delta), 10)
}

func (h *colorHandler) writeGroups(buf *buffer) {
	last := len(h.groups) - 1
	for i, group := range h.groups {
		if i == 0 {
			if group == pluginGroupPrefix {
				buf.WriteStringIf(h.colorEnabled, ansiBrightCyan)
			} else {
				buf.WriteStringIf(h.colorEnabled, ansiBrightGreen)
			}
		}
		buf.WriteString(group)
		if i < last {
			buf.WriteByte('.')
		} else {
			buf.WriteByte(' ')
		}
	}
	buf.WriteStringIf(h.colorEnabled, ansiReset)
}

func (h *colorHandler) writeSource(buf *buffer, pc uintptr) {
	fs := runtime.CallersFrames([]uintptr{pc})
	f, _ := fs.Next()
	if f.File != "" {
		if h.replaceAttr == nil {
			h.appendSource(buf, f.File, f.Line)
			buf.WriteByte(' ')
		} else if a := h.replaceAttr(nil /* groups */, slog.Any(slog.SourceKey, &slog.Source{
			Function: f.Function,
			File:     f.File,
			Line:     f.Line,
		})); a.Key != "" {
			h.appendValue(buf, a.Value, false)
			buf.WriteByte(' ')
		}
	}
}

func (h *colorHandler) appendSource(buf *buffer, file string, line int) {
	dir, file := filepath.Split(file)

	buf.WriteStringIf(h.colorEnabled, ansiFaintStyle)
	buf.WriteString(filepath.Base(dir))
	buf.WriteByte('/')
	buf.WriteString(file)
	buf.WriteByte(':')
	*buf = strconv.AppendInt(*buf, int64(line), 10)
	buf.WriteStringIf(h.colorEnabled, ansiResetFaintStyle)
}

func (h *colorHandler) writeMessage(buf *buffer, msg string) {
	if h.replaceAttr == nil {
		buf.WriteString(msg)
		buf.WriteByte(' ')
	} else if a := h.replaceAttr(nil /* groups */, slog.String(slog.MessageKey, msg)); a.Key != "" {
		h.appendValue(buf, a.Value, false)
		buf.WriteByte(' ')
	}
}

func (h *colorHandler) appendAttr(buf *buffer, attr slog.Attr, groupsPrefix string) {
	if attr.Equal(slog.Attr{}) {
		return
	}
	attr.Value = attr.Value.Resolve()

	switch attr.Value.Kind() {
	case slog.KindGroup:
		if attr.Key != "" {
			groupsPrefix += attr.Key + "."
		}
		for _, groupAttr := range attr.Value.Group() {
			h.appendAttr(buf, groupAttr, groupsPrefix)
		}
	case slog.KindAny:
		if e, ok := attr.Value.Any().(noAllocErr); ok {
			h.appendErr(buf, e)
			buf.WriteByte(' ')
			break
		}
		fallthrough
	default:
		h.appendKey(buf, attr.Key)
		h.appendValue(buf, attr.Value, true)
		buf.WriteByte(' ')
	}
}

func (h *colorHandler) appendKey(buf *buffer, key string) {
	buf.WriteStringIf(h.colorEnabled, ansiFaintStyle)
	h.appendString(buf, key, true)
	buf.WriteByte('=')
	buf.WriteStringIf(h.colorEnabled, ansiReset)
}

func (h *colorHandler) appendValue(buf *buffer, v slog.Value, shouldQuote bool) {
	switch v.Kind() {
	case slog.KindString:
		h.appendString(buf, v.String(), shouldQuote)
	case slog.KindInt64:
		*buf = strconv.AppendInt(*buf, v.Int64(), 10)
	case slog.KindUint64:
		*buf = strconv.AppendUint(*buf, v.Uint64(), 10)
	case slog.KindFloat64:
		*buf = strconv.AppendFloat(*buf, v.Float64(), 'g', -1, 64)
	case slog.KindBool:
		*buf = strconv.AppendBool(*buf, v.Bool())
	case slog.KindDuration:
		h.appendString(buf, v.Duration().String(), shouldQuote)
	case slog.KindTime:
		*buf = v.Time().AppendFormat(*buf, h.timeFormat)
	case slog.KindAny:
		switch cv := v.Any().(type) {
		case slog.Level:
			h.appendLevel(buf, cv)
		case encoding.TextMarshaler:
			data, err := cv.MarshalText()
			if err != nil {
				break
			}
			h.appendString(buf, string(data), shouldQuote)
		case *slog.Source:
			h.appendSource(buf, cv.File, cv.Line)
		default:
			h.appendString(buf, fmt.Sprint(v.Any()), shouldQuote)
		}
	}
}

func (h *colorHandler) appendErr(buf *buffer, err error) {
	buf.WriteStringIf(h.colorEnabled, ansiBrightRedFaint)
	buf.WriteString(errKey)
	buf.WriteByte('=')
	buf.WriteStringIf(h.colorEnabled, ansiResetFaintStyle)
	h.appendString(buf, err.Error(), true)
	buf.WriteStringIf(h.colorEnabled, ansiReset)
}

func (h *colorHandler) appendString(buf *buffer, s string, shouldQuote bool) {
	if shouldQuote && needsQuoting(s) {
		*buf = strconv.AppendQuote(*buf, s)
	} else {
		buf.WriteString(s)
	}
}

func needsQuoting(s string) bool {
	if len(s) == 0 {
		return true
	}
	for _, r := range s {
		if unicode.IsSpace(r) || r == '"' || r == '=' || !unicode.IsPrint(r) {
			return true
		}
	}
	return false
}

type noAllocErr struct{ error }
