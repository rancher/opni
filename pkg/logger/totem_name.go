package logger

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"strings"

	"github.com/charmbracelet/lipgloss"
	gsync "github.com/kralicky/gpkg/sync"
	slogmulti "github.com/samber/slog-multi"
)

var (
	colorCache gsync.Map[string, lipgloss.Style]
)

func init() {
	colorCache.Store("totem", lipgloss.NewStyle().Background(lipgloss.Color("15")).Foreground(lipgloss.Color("0")))

}

func newRandomForegroundStyle() lipgloss.Style {
	r := math.Round(rand.Float64()*127) + 127
	g := math.Round(rand.Float64()*127) + 127
	b := math.Round(rand.Float64()*127) + 127
	return lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", int(r), int(g), int(b))))
}

func newTotemNameMiddleware() slogmulti.Middleware {
	return func(next slog.Handler) slog.Handler {
		return &totemNameMiddleware{
			next: next,
		}
	}
}

type totemNameMiddleware struct {
	next   slog.Handler
	groups []string
}

func (h *totemNameMiddleware) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *totemNameMiddleware) Handle(ctx context.Context, record slog.Record) error {
	attrs := []slog.Attr{}

	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr)
		return true
	})

	sb := strings.Builder{}
	for i, part := range h.groups {
		if c, ok := colorCache.Load(part); !ok {
			newStyle := newRandomForegroundStyle()
			colorCache.Store(part, newStyle)
			sb.WriteString(newStyle.Render(part))
		} else {
			sb.WriteString(c.Render(part))
		}
		if i < len(h.groups)-1 {
			sb.WriteString(".")
		} else {
			sb.WriteByte(' ')
		}
	}

	// insert logger name before message
	sb.WriteString(record.Message)

	record = slog.NewRecord(record.Time, record.Level, sb.String(), record.PC)
	record.AddAttrs(attrs...)

	return h.next.Handle(ctx, record)
}

func (h *totemNameMiddleware) WithAttrs(attrs []slog.Attr) slog.Handler {

	return &totemNameMiddleware{
		next:   h.next.WithAttrs(attrs),
		groups: h.groups,
	}
}

func (h *totemNameMiddleware) WithGroup(name string) slog.Handler {
	return &totemNameMiddleware{
		next:   h.next.WithGroup(name),
		groups: append(h.groups, name),
	}
}
