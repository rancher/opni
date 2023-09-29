package logger

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// modified from https://github.com/lmittmann/tint to match colorHandler's expected behaviour

var faketime = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)

func Example() {
	slog.SetDefault(slog.New(newColorHandler(os.Stderr, &LoggerOptions{
		Level:        slog.LevelDebug,
		ColorEnabled: true,
	})))

	slog.Info("Starting server", "addr", ":8080", "env", "production")
	slog.Debug("Connected to DB", "db", "myapp", "host", "localhost:5432")
	slog.Warn("Slow request", "method", "GET", "path", "/users", "duration", 497*time.Millisecond)
	slog.Error("DB connection lost", Err(errors.New("connection reset")), "db", "myapp")
	// Output:
}

// Run test with "faketime" tag:
//
// TZ="" go test -tags=faketime
func TestHandler(t *testing.T) {
	if !faketime.Equal(time.Now()) {
		t.Skip(`skipping test; run with "-tags=faketime"`)
	}

	tests := []struct {
		Opts *LoggerOptions
		F    func(l *slog.Logger)
		Want string
	}{
		{
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `2009 Nov 10 23:00:00 INFO test key=val`,
		},
		{
			F: func(l *slog.Logger) {
				l.Error("test", Err(errors.New("fail")))
			},
			Want: `2009 Nov 10 23:00:00 ERROR test err=fail`,
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", slog.Group("group", slog.String("key", "val"), Err(errors.New("fail"))))
			},
			Want: `2009 Nov 10 23:00:00 INFO test key=val err=fail`,
		},
		{
			F: func(l *slog.Logger) {
				l.WithGroup("group").Info("test", "key", "val")
			},
			Want: `2009 Nov 10 23:00:00 INFO group test key=val`,
		},
		{
			F: func(l *slog.Logger) {
				l.With("key", "val").Info("test", "key2", "val2")
			},
			Want: `2009 Nov 10 23:00:00 INFO test key=val key2=val2`,
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", "k e y", "v a l")
			},
			Want: `2009 Nov 10 23:00:00 INFO test "k e y"="v a l"`,
		},
		{
			F: func(l *slog.Logger) {
				l.WithGroup("g r o u p").Info("test", "key", "val")
			},
			Want: `2009 Nov 10 23:00:00 INFO g r o u p test key=val`,
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", "slice", []string{"a", "b", "c"}, "map", map[string]int{"a": 1, "b": 2, "c": 3})
			},
			Want: `2009 Nov 10 23:00:00 INFO test slice="[a b c]" map="map[a:1 b:2 c:3]"`,
		},
		{
			Opts: &LoggerOptions{
				AddSource: true,
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `2009 Nov 10 23:00:00 INFO logger/color_handler_test.go:98 test key=val`,
		},
		{
			Opts: &LoggerOptions{
				DisableTime: true,
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `INFO test key=val`,
		},
		{
			Opts: &LoggerOptions{
				ReplaceAttr: drop(slog.TimeKey),
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `INFO test key=val`,
		},
		{
			Opts: &LoggerOptions{
				ReplaceAttr: drop(slog.LevelKey),
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `2009 Nov 10 23:00:00 test key=val`,
		},
		{
			Opts: &LoggerOptions{
				ReplaceAttr: drop(slog.MessageKey),
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `2009 Nov 10 23:00:00 INFO key=val`,
		},
		{
			Opts: &LoggerOptions{
				ReplaceAttr: drop(slog.TimeKey, slog.LevelKey, slog.MessageKey),
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `key=val`,
		},
		{
			Opts: &LoggerOptions{
				ReplaceAttr: drop("key"),
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `2009 Nov 10 23:00:00 INFO test`,
		},
		{
			Opts: &LoggerOptions{
				ReplaceAttr: drop("key"),
			},
			F: func(l *slog.Logger) {
				l.WithGroup("group").Info("test", "key", "val", "key2", "val2")
			},
			Want: `2009 Nov 10 23:00:00 INFO group test key=val key2=val2`,
		},
		{
			Opts: &LoggerOptions{
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					if a.Key == "key" && len(groups) == 1 && groups[0] == "group" {
						return slog.Attr{}
					}
					return a
				},
			},
			F: func(l *slog.Logger) {
				l.WithGroup("group").Info("test", "key", "val", "key2", "val2")
			},
			Want: `2009 Nov 10 23:00:00 INFO group test key2=val2`,
		},
		{
			Opts: &LoggerOptions{
				ReplaceAttr: replace(slog.IntValue(42), slog.TimeKey),
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `42 INFO test key=val`,
		},
		{
			Opts: &LoggerOptions{
				ReplaceAttr: replace(slog.StringValue("INF"), slog.LevelKey),
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `2009 Nov 10 23:00:00 INF test key=val`,
		},
		{
			Opts: &LoggerOptions{
				ReplaceAttr: replace(slog.IntValue(42), slog.MessageKey),
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: `2009 Nov 10 23:00:00 INFO 42 key=val`,
		},
		{
			Opts: &LoggerOptions{
				ReplaceAttr: replace(slog.IntValue(42), "key"),
			},
			F: func(l *slog.Logger) {
				l.With("key", "val").Info("test", "key2", "val2")
			},
			Want: `2009 Nov 10 23:00:00 INFO test key=42 key2=val2`,
		},
		{
			Opts: &LoggerOptions{
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					return slog.Attr{}
				},
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: ``,
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", "key", "")
			},
			Want: `2009 Nov 10 23:00:00 INFO test key=""`,
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", "", "val")
			},
			Want: `2009 Nov 10 23:00:00 INFO test ""=val`,
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", "", "")
			},
			Want: `2009 Nov 10 23:00:00 INFO test ""=""`,
		},

		{
			F: func(l *slog.Logger) {
				l.Log(context.TODO(), slog.LevelInfo+1, "test")
			},
			Want: `2009 Nov 10 23:00:00 INFO+1 test`,
		},
		{
			Opts: &LoggerOptions{
				Level: slog.LevelDebug - 1,
			},
			F: func(l *slog.Logger) {
				l.Log(context.TODO(), slog.LevelDebug-1, "test")
			},
			Want: `2009 Nov 10 23:00:00 DEBUG-1 test`,
		},
		{
			F: func(l *slog.Logger) {
				l.Error("test", slog.Any("error", errors.New("fail")))
			},
			Want: `2009 Nov 10 23:00:00 ERROR test error=fail`,
		},
		{
			F: func(l *slog.Logger) {
				l.Error("test", Err(nil))
			},
			Want: `2009 Nov 10 23:00:00 ERROR test err=<nil>`,
		},
		{
			Opts: &LoggerOptions{
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					if a.Key == slog.TimeKey && len(groups) == 0 {
						return slog.Time(slog.TimeKey, a.Value.Time().Add(24*time.Hour))
					}
					return a
				},
			},
			F: func(l *slog.Logger) {
				l.Error("test")
			},
			Want: `2009 Nov 11 23:00:00 ERROR test`,
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", "a", "b", slog.Group("", slog.String("c", "d")), "e", "f")
			},
			Want: `2009 Nov 10 23:00:00 INFO test a=b c=d e=f`,
		},
		{
			Opts: &LoggerOptions{
				ReplaceAttr: drop(slog.TimeKey, slog.LevelKey, slog.MessageKey, slog.SourceKey),
				AddSource:   true,
			},
			F: func(l *slog.Logger) {
				l.WithGroup("group").Info("test", "key", "val")
			},
			Want: `group key=val`,
		},
		{
			Opts: &LoggerOptions{
				ReplaceAttr: func(g []string, a slog.Attr) slog.Attr {
					if len(g) == 0 && a.Key == slog.LevelKey {
						_ = a.Value.Any().(slog.Level)
					}
					return a
				},
			},
			F: func(l *slog.Logger) {
				l.Info("test")
			},
			Want: `2009 Nov 10 23:00:00 INFO test`,
		},
		{
			Opts: &LoggerOptions{
				AddSource: true,
				ReplaceAttr: func(g []string, a slog.Attr) slog.Attr {
					return a
				},
			},
			F: func(l *slog.Logger) {
				l.Info("test")
			},
			Want: `2009 Nov 10 23:00:00 INFO logger/color_handler_test.go:324 test`,
		},
		{
			F: func(l *slog.Logger) {
				l = l.WithGroup("group")
				l.Error("test", Err(errTest))
			},
			Want: `2009 Nov 10 23:00:00 ERROR group test err=fail`,
		},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			var buf bytes.Buffer
			if test.Opts == nil {
				test.Opts = &LoggerOptions{}
			}

			test.Opts.ColorEnabled = false
			l := slog.New(newColorHandler(&buf, test.Opts))
			test.F(l)

			got := strings.TrimRight(buf.String(), "\n")
			if test.Want != got {
				t.Fatalf("(-want +got)\n- %s\n+ %s", test.Want, got)
			}
		})
	}
}

// drop returns a ReplaceAttr that drops the given keys.
// ignores groups as colorHandler does not prefix keys with groups
func drop(keys ...string) func([]string, slog.Attr) slog.Attr {
	return func(groups []string, a slog.Attr) slog.Attr {

		for _, key := range keys {
			if a.Key == key {
				a = slog.Attr{}
			}
		}
		return a
	}
}

func replace(new slog.Value, keys ...string) func([]string, slog.Attr) slog.Attr {
	return func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) > 0 {
			return a
		}

		for _, key := range keys {
			if a.Key == key {
				a.Value = new
			}
		}
		return a
	}
}

// See https://github.com/golang/exp/blob/master/slog/benchmarks/benchmarks_test.go#L25
//
// Run e.g.:
//
//	go test -bench=. -count=10 | benchstat -col /h /dev/stdin
func BenchmarkLogAttrs(b *testing.B) {
	handler := []struct {
		Name string
		H    slog.Handler
	}{
		{"colorHandler", newColorHandler(io.Discard, nil)},
		{"text", slog.NewTextHandler(io.Discard, nil)},
		{"json", slog.NewJSONHandler(io.Discard, nil)},
		{"discard", new(discarder)},
	}

	benchmarks := []struct {
		Name string
		F    func(*slog.Logger)
	}{
		{
			"5 args",
			func(logger *slog.Logger) {
				logger.LogAttrs(context.TODO(), slog.LevelInfo, testMessage,
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest))
			},
		},
		{
			"5 args custom level",
			func(logger *slog.Logger) {
				logger.LogAttrs(context.TODO(), slog.LevelInfo+1, testMessage,
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
				)
			},
		},
		{
			"10 args",
			func(logger *slog.Logger) {
				logger.LogAttrs(context.TODO(), slog.LevelInfo, testMessage,
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
				)
			},
		},
		{
			"40 args",
			func(logger *slog.Logger) {
				logger.LogAttrs(context.TODO(), slog.LevelInfo, testMessage,
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("error", errTest),
				)
			},
		},
	}

	for _, h := range handler {
		b.Run("h="+h.Name, func(b *testing.B) {
			for _, bench := range benchmarks {
				b.Run(bench.Name, func(b *testing.B) {
					b.ReportAllocs()
					logger := slog.New(h.H)
					for i := 0; i < b.N; i++ {
						bench.F(logger)
					}
				})
			}
		})
	}
}

// discarder is a slog.Handler that discards all records.
type discarder struct{}

func (*discarder) Enabled(context.Context, slog.Level) bool   { return true }
func (*discarder) Handle(context.Context, slog.Record) error  { return nil }
func (d *discarder) WithAttrs(attrs []slog.Attr) slog.Handler { return d }
func (d *discarder) WithGroup(name string) slog.Handler       { return d }

var (
	testMessage  = "Test logging, but use a somewhat realistic message length."
	testTime     = time.Date(2022, time.May, 1, 0, 0, 0, 0, time.UTC)
	testString   = "7e3b3b2aaeff56a7108fe11e154200dd/7819479873059528190"
	testInt      = 32768
	testDuration = 23 * time.Second
	errTest      = errors.New("fail")
)
