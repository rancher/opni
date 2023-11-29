package logger

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// modified from https://github.com/lmittmann/tint to match protoHandler's expected behaviour

// Run test with "faketime" tag:
//
// TZ="" go test -tags=faketime
func TestProtoHandler(t *testing.T) {
	slog.SetDefault(slog.New(newProtoHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	if !faketime.Equal(time.Now()) {
		t.Skip(`skipping test; run with "-tags=faketime"`)
	}

	tests := []struct {
		Opts *slog.HandlerOptions
		F    func(l *slog.Logger)
		Want *controlv1.StructuredLogRecord
	}{
		{
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: toStructuredLog(faketime, "INFO", "", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "key",
						Value: "val",
					},
				}),
		},
		{
			F: func(l *slog.Logger) {
				l.Error("test", Err(errors.New("fail")))
			},
			Want: toStructuredLog(faketime, "ERROR", "", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "err",
						Value: "fail",
					},
				}),
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", slog.Group("group", slog.String("key", "val"), Err(errors.New("fail"))))
			},
			Want: toStructuredLog(faketime, "INFO", "", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "key",
						Value: "val",
					},
					{
						Key:   "err",
						Value: "fail",
					},
				}),
		},
		{
			F: func(l *slog.Logger) {
				l.WithGroup("group").Info("test", "key", "val")
			},
			Want: toStructuredLog(faketime, "INFO", "group", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "key",
						Value: "val",
					},
				}),
		},
		{
			F: func(l *slog.Logger) {
				l.With("key", "val").Info("test", "key2", "val2")
			},
			Want: toStructuredLog(faketime, "INFO", "", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "key",
						Value: "val",
					},
					{
						Key:   "key2",
						Value: "val2",
					},
				}),
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", "k e y", "v a l")
			},
			Want: toStructuredLog(faketime, "INFO", "", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "k e y",
						Value: "v a l",
					},
				}),
		},
		{
			F: func(l *slog.Logger) {
				l.WithGroup("g r o u p").Info("test", "key", "val")
			},
			Want: toStructuredLog(faketime, "INFO", "g r o u p", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "key",
						Value: "val",
					},
				}),
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", "slice", []string{"a", "b", "c"}, "map", map[string]int{"a": 1, "b": 2, "c": 3})
			},
			Want: toStructuredLog(faketime, "INFO", "", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "slice",
						Value: "[a b c]",
					},
					{
						Key:   "map",
						Value: "map[a:1 b:2 c:3]",
					},
				}),
		},
		{
			Opts: &slog.HandlerOptions{
				AddSource: true,
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: toStructuredLog(faketime, "INFO", "", "logger/proto_handler_test.go:151", "test",
				[]*controlv1.Attr{
					{
						Key:   "key",
						Value: "val",
					},
				}),
		},
		{
			Opts: &slog.HandlerOptions{
				ReplaceAttr: drop(slog.TimeKey),
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: toStructuredLog(time.Time{}, "INFO", "", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "key",
						Value: "val",
					},
				}),
		},
		{
			Opts: &slog.HandlerOptions{
				ReplaceAttr: drop(slog.LevelKey),
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: toStructuredLog(faketime, "", "", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "key",
						Value: "val",
					},
				}),
		},
		{
			Opts: &slog.HandlerOptions{
				ReplaceAttr: drop(slog.MessageKey),
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: toStructuredLog(faketime, "INFO", "", "", "",
				[]*controlv1.Attr{
					{
						Key:   "key",
						Value: "val",
					},
				}),
		},
		{
			Opts: &slog.HandlerOptions{
				ReplaceAttr: drop(slog.TimeKey, slog.LevelKey, slog.MessageKey),
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: toStructuredLog(time.Time{}, "", "", "", "",
				[]*controlv1.Attr{
					{
						Key:   "key",
						Value: "val",
					},
				}),
		},
		{
			Opts: &slog.HandlerOptions{
				ReplaceAttr: drop("key"),
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: toStructuredLog(faketime, "INFO", "", "", "test",
				[]*controlv1.Attr{}),
		},
		{
			Opts: &slog.HandlerOptions{
				ReplaceAttr: drop("key"),
			},
			F: func(l *slog.Logger) {
				l.WithGroup("group").Info("test", "key", "val", "key2", "val2")
			},
			Want: toStructuredLog(faketime, "INFO", "group", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "key2",
						Value: "val2",
					},
				}),
		},
		{
			Opts: &slog.HandlerOptions{
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
			Want: toStructuredLog(faketime, "INFO", "group", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "key2",
						Value: "val2",
					},
				}),
		},
		{
			Opts: &slog.HandlerOptions{
				ReplaceAttr: replace(slog.StringValue("INF"), slog.LevelKey),
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: toStructuredLog(faketime, "INF", "", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "key",
						Value: "val",
					},
				}),
		},
		{
			Opts: &slog.HandlerOptions{
				ReplaceAttr: replace(slog.IntValue(42), slog.MessageKey),
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: toStructuredLog(faketime, "INFO", "", "", "42",
				[]*controlv1.Attr{
					{
						Key:   "key",
						Value: "val",
					},
				}),
		},
		{
			Opts: &slog.HandlerOptions{
				ReplaceAttr: replace(slog.IntValue(42), "key"),
			},
			F: func(l *slog.Logger) {
				l.With("key", "val").Info("test", "key2", "val2")
			},
			Want: toStructuredLog(faketime, "INFO", "", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "key",
						Value: "42",
					},
					{
						Key:   "key2",
						Value: "val2",
					},
				}),
		},
		{
			Opts: &slog.HandlerOptions{
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					return slog.Attr{}
				},
			},
			F: func(l *slog.Logger) {
				l.Info("test", "key", "val")
			},
			Want: toStructuredLog(time.Time{}, "", "", "", "",
				[]*controlv1.Attr{}),
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", "key", "")
			},
			Want: toStructuredLog(faketime, "INFO", "", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "key",
						Value: "",
					},
				}),
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", "", "val")
			},
			Want: toStructuredLog(faketime, "INFO", "", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "",
						Value: "val",
					},
				}),
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", "", "")
			},
			Want: toStructuredLog(faketime, "INFO", "", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "",
						Value: "",
					},
				}),
		},

		{
			F: func(l *slog.Logger) {
				l.Log(context.TODO(), slog.LevelInfo+1, "test")
			},
			Want: toStructuredLog(faketime, "INFO+1", "", "", "test",
				[]*controlv1.Attr{}),
		},
		{
			Opts: &slog.HandlerOptions{
				Level: slog.LevelDebug - 1,
			},
			F: func(l *slog.Logger) {
				l.Log(context.TODO(), slog.LevelDebug-1, "test")
			},
			Want: toStructuredLog(faketime, "DEBUG-1", "", "", "test",
				[]*controlv1.Attr{}),
		},
		{
			F: func(l *slog.Logger) {
				l.Error("test", slog.Any("ERROR", errors.New("fail")))
			},
			Want: toStructuredLog(faketime, "ERROR", "", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "ERROR",
						Value: "fail",
					},
				}),
		},
		{
			F: func(l *slog.Logger) {
				l.Error("test", Err(nil))
			},
			Want: toStructuredLog(faketime, "ERROR", "", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "err",
						Value: "<nil>",
					},
				}),
		},
		{
			Opts: &slog.HandlerOptions{
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
			Want: toStructuredLog(faketime.Add(24*time.Hour), "ERROR", "", "", "test",
				[]*controlv1.Attr{}),
		},
		{
			F: func(l *slog.Logger) {
				l.Info("test", "a", "b", slog.Group("", slog.String("c", "d")), "e", "f")
			},
			Want: toStructuredLog(faketime, "INFO", "", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "a",
						Value: "b",
					},
					{
						Key:   "c",
						Value: "d",
					},
					{
						Key:   "e",
						Value: "f",
					},
				}),
		},
		{
			Opts: &slog.HandlerOptions{
				ReplaceAttr: drop(slog.TimeKey, slog.LevelKey, slog.MessageKey, slog.SourceKey),
				AddSource:   true,
			},
			F: func(l *slog.Logger) {
				l.WithGroup("group").Info("test", "key", "val")
			},
			Want: toStructuredLog(time.Time{}, "", "group", "", "",
				[]*controlv1.Attr{
					{
						Key:   "key",
						Value: "val",
					},
				}),
		},
		{
			Opts: &slog.HandlerOptions{
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
			Want: toStructuredLog(faketime, "INFO", "", "", "test",
				[]*controlv1.Attr{}),
		},
		{
			Opts: &slog.HandlerOptions{
				AddSource: true,
				ReplaceAttr: func(g []string, a slog.Attr) slog.Attr {
					return a
				},
			},
			F: func(l *slog.Logger) {
				l.Info("test")
			},
			Want: toStructuredLog(faketime, "INFO", "", "logger/proto_handler_test.go:479", "test",
				[]*controlv1.Attr{}),
		},
		{
			F: func(l *slog.Logger) {
				l = l.WithGroup("group")
				l.Error("test", Err(errTest))
			},
			Want: toStructuredLog(faketime, "ERROR", "group", "", "test",
				[]*controlv1.Attr{
					{
						Key:   "err",
						Value: "fail",
					},
				}),
		},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			var buf bytes.Buffer
			if test.Opts == nil {
				test.Opts = &slog.HandlerOptions{
					Level: slog.LevelDebug,
				}
			}

			l := slog.New(newProtoHandler(&buf, test.Opts))
			test.F(l)

			sizeBytes := buf.Next(4)
			size := int(sizeBytes[0]) |
				int(sizeBytes[1])<<8 |
				int(sizeBytes[2])<<16 |
				int(sizeBytes[3])<<24
			recordBytes := buf.Next(size)
			got := &controlv1.StructuredLogRecord{}

			if err := proto.Unmarshal(recordBytes, got); err != nil {
				t.Errorf("failed to unmarshal log record bytes: %s", err.Error())
			}

			if !proto.Equal(test.Want, got) {
				t.Fatalf("(-want +got)\n- %s\n+ %s", test.Want, got)
			}
		})
	}
}

func toStructuredLog(t time.Time, level string, name string, src string, msg string, attrs []*controlv1.Attr) *controlv1.StructuredLogRecord {
	timestamppb := timestamppb.New(t)
	if t == (time.Time{}) {
		timestamppb = nil
	}
	return &controlv1.StructuredLogRecord{
		Time:       timestamppb,
		Message:    msg,
		Name:       name,
		Source:     src,
		Level:      level,
		Attributes: attrs,
	}
}

// See https://github.com/golang/exp/blob/master/slog/benchmarks/benchmarks_test.go#L25
//
// Run e.g.:
//
//	go test -bench=BenchmarkProtoLogAttrs -count=10 | benchstat -col /h /dev/stdin
func BenchmarkProtoLogAttrs(b *testing.B) {
	handler := []struct {
		Name string
		H    slog.Handler
	}{
		{"protoHandler", newProtoHandler(io.Discard, &slog.HandlerOptions{AddSource: false})}, // slog handlers omit source by default
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
					slog.Any("ERROR", errTest))
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
					slog.Any("ERROR", errTest),
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
					slog.Any("ERROR", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("ERROR", errTest),
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
					slog.Any("ERROR", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("ERROR", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("ERROR", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("ERROR", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("ERROR", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("ERROR", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("ERROR", errTest),
					slog.String("string", testString),
					slog.Int("status", testInt),
					slog.Duration("duration", testDuration),
					slog.Time("time", testTime),
					slog.Any("ERROR", errTest),
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
