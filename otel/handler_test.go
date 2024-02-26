// Copyright (c) 2024 The sloth authors
// Use of this source code is governed by a MIT license found in the LICENSE file.

package otel_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/nil-go/sloth/otel"
	"github.com/nil-go/sloth/otel/internal/assert"
)

func TestNew_panic(t *testing.T) {
	t.Parallel()

	defer func() {
		assert.Equal(t, "cannot create Handler with nil handler", recover().(string))
	}()

	otel.New(nil)
	t.Fail()
}

func record(level slog.Level, message string, attrs ...any) slog.Record {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:])

	record := slog.NewRecord(time.Unix(100, 1000), level, message, pcs[0])
	record.Add(attrs...)

	return record
}

func TestHandler(t *testing.T) {
	t.Parallel()

	for _, testcase := range testcases() {
		testcase := testcase

		t.Run(testcase.description, func(t *testing.T) {
			t.Parallel()

			span := &spanStub{
				recording:   testcase.recording,
				spanContext: testcase.spanContext,
			}
			ctx := trace.ContextWithSpan(context.Background(), span)

			buf := &bytes.Buffer{}
			handler := otel.New(slog.NewTextHandler(buf, &slog.HandlerOptions{
				ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
					if len(groups) == 0 && attr.Key == slog.TimeKey {
						return slog.Attr{}
					}

					return attr
				},
			}), testcase.opts...)
			assert.NoError(t, handler.WithAttrs([]slog.Attr{slog.String("a", "A")}).
				Handle(ctx, record(testcase.level, "msg1")))
			gHandler := handler.WithGroup("g")
			assert.NoError(t, gHandler.WithAttrs([]slog.Attr{slog.String("b", "B")}).
				Handle(ctx, record(testcase.level, "msg2")))
			assert.NoError(t, gHandler.WithGroup("h").
				Handle(ctx, record(testcase.level, "msg3", "error", errors.New("an error"))))

			assert.Equal(t, testcase.expectedLog, buf.String())
			assert.Equal(t, testcase.expectedSpan.events, span.events)
			assert.Equal(t, fmt.Sprintf("%v", testcase.expectedSpan.errors), fmt.Sprintf("%v", span.errors))
			assert.Equal(t, testcase.expectedSpan.status, span.status)
			assert.Equal(t, testcase.expectedSpan.message, span.message)
		})
	}
}

//nolint:lll
func testcases() []struct {
	description  string
	level        slog.Level
	spanContext  trace.SpanContext
	recording    bool
	opts         []otel.Option
	expectedLog  string
	expectedSpan spanStub
} {
	path, _ := os.Getwd()
	filePath := semconv.CodeFilepath(path + "/handler_test.go")
	function := semconv.CodeFunction("github.com/nil-go/sloth/otel_test.TestHandler.func1")

	return []struct {
		description  string
		level        slog.Level
		spanContext  trace.SpanContext
		recording    bool
		opts         []otel.Option
		expectedLog  string
		expectedSpan spanStub
	}{
		{
			description: "invalid span context",
			expectedLog: `level=INFO msg=msg1 a=A
level=INFO msg=msg2 g.b=B
level=INFO msg=msg3 g.h.error="an error"
`,
		},
		{
			description: "trace context",
			spanContext: trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    [16]byte{75, 249, 47, 53, 119, 179, 77, 166, 163, 206, 146, 157, 14, 14, 71, 54},
				SpanID:     [8]byte{0, 240, 103, 170, 11, 169, 2, 183},
				TraceFlags: trace.TraceFlags(0),
			}),
			expectedLog: `level=INFO msg=msg1 a=A trace-id=4bf92f3577b34da6a3ce929d0e0e4736 span-id=00f067aa0ba902b7 trace-flags=00
level=INFO msg=msg2 trace-id=4bf92f3577b34da6a3ce929d0e0e4736 span-id=00f067aa0ba902b7 trace-flags=00 g.b=B
level=INFO msg=msg3 trace-id=4bf92f3577b34da6a3ce929d0e0e4736 span-id=00f067aa0ba902b7 trace-flags=00 g.h.error="an error"
`,
		},
		{
			description: "without record event",
			spanContext: trace.NewSpanContext(trace.SpanContextConfig{
				TraceFlags: trace.TraceFlags(1),
			}),
			recording: true,
			expectedLog: `level=INFO msg=msg1 a=A
level=INFO msg=msg2 g.b=B
level=INFO msg=msg3 g.h.error="an error"
`,
		},
		{
			description: "not recording",
			spanContext: trace.NewSpanContext(trace.SpanContextConfig{
				TraceFlags: trace.TraceFlags(1),
			}),
			opts: []otel.Option{
				otel.WithRecordEvent(false),
			},
			expectedLog: `level=INFO msg=msg1 a=A
level=INFO msg=msg2 g.b=B
level=INFO msg=msg3 g.h.error="an error"
`,
		},
		{
			description: "not sampled",
			spanContext: trace.NewSpanContext(trace.SpanContextConfig{
				TraceFlags: trace.TraceFlags(0),
			}),
			recording: true,
			opts: []otel.Option{
				otel.WithRecordEvent(false),
			},
			expectedLog: `level=INFO msg=msg1 a=A
level=INFO msg=msg2 g.b=B
level=INFO msg=msg3 g.h.error="an error"
`,
		},
		{
			description: "with record event (info)",
			spanContext: trace.NewSpanContext(trace.SpanContextConfig{
				TraceFlags: trace.TraceFlags(1),
			}),
			recording: true,
			opts: []otel.Option{
				otel.WithRecordEvent(false),
			},
			expectedSpan: spanStub{
				events: map[string][]trace.EventOption{
					"msg1": {
						trace.WithTimestamp(time.Unix(100, 1000)),
						trace.WithAttributes(attribute.String("a", "A"), filePath, semconv.CodeLineNumber(73), function),
					},
					"msg2": {
						trace.WithTimestamp(time.Unix(100, 1000)),
						trace.WithAttributes(attribute.String("g.b", "B"), filePath, semconv.CodeLineNumber(76), function),
					},
					"msg3": {
						trace.WithTimestamp(time.Unix(100, 1000)),
						trace.WithAttributes(filePath, semconv.CodeLineNumber(78), function, attribute.String("g.h.error", "an error")),
					},
				},
			},
		},
		{
			description: "with record event (error)",
			level:       slog.LevelError,
			spanContext: trace.NewSpanContext(trace.SpanContextConfig{
				TraceFlags: trace.TraceFlags(1),
			}),
			recording: true,
			opts: []otel.Option{
				otel.WithRecordEvent(false),
			},
			expectedSpan: spanStub{
				errors: map[error][]trace.EventOption{
					errors.New("msg1"): {
						trace.WithTimestamp(time.Unix(100, 1000)),
						trace.WithAttributes(attribute.String("a", "A"), filePath, semconv.CodeLineNumber(73), function),
					},
					errors.New("msg2"): {
						trace.WithTimestamp(time.Unix(100, 1000)),
						trace.WithAttributes(attribute.String("g.b", "B"), filePath, semconv.CodeLineNumber(76), function),
					},
					fmt.Errorf("msg3: %w", errors.New("an error")): {
						trace.WithTimestamp(time.Unix(100, 1000)),
						trace.WithAttributes(filePath, semconv.CodeLineNumber(78), function),
					},
				},
				status:  codes.Error,
				message: "msg3",
			},
		},
		{
			description: "pass through",
			spanContext: trace.NewSpanContext(trace.SpanContextConfig{
				TraceFlags: trace.TraceFlags(1),
			}),
			recording: true,
			opts: []otel.Option{
				otel.WithRecordEvent(true),
			},
			expectedLog: `level=INFO msg=msg1 a=A
level=INFO msg=msg2 g.b=B
level=INFO msg=msg3 g.h.error="an error"
`,
			expectedSpan: spanStub{
				events: map[string][]trace.EventOption{
					"msg1": {
						trace.WithTimestamp(time.Unix(100, 1000)),
						trace.WithAttributes(attribute.String("a", "A"), filePath, semconv.CodeLineNumber(73), function),
					},
					"msg2": {
						trace.WithTimestamp(time.Unix(100, 1000)),
						trace.WithAttributes(attribute.String("g.b", "B"), filePath, semconv.CodeLineNumber(76), function),
					},
					"msg3": {
						trace.WithTimestamp(time.Unix(100, 1000)),
						trace.WithAttributes(filePath, semconv.CodeLineNumber(78), function, attribute.String("g.h.error", "an error")),
					},
				},
			},
		},
	}
}

type spanStub struct {
	trace.Span

	recording   bool
	spanContext trace.SpanContext

	events  map[string][]trace.EventOption
	errors  map[error][]trace.EventOption
	status  codes.Code
	message string
}

func (s *spanStub) AddEvent(name string, options ...trace.EventOption) {
	if s.events == nil {
		s.events = make(map[string][]trace.EventOption)
	}
	s.events[name] = options
}

func (s *spanStub) RecordError(err error, options ...trace.EventOption) {
	if s.errors == nil {
		s.errors = make(map[error][]trace.EventOption)
	}
	s.errors[err] = options
}

func (s *spanStub) SetStatus(status codes.Code, message string) {
	s.status = status
	s.message = message
}

func (s *spanStub) IsRecording() bool {
	return s.recording
}

func (s *spanStub) SpanContext() trace.SpanContext {
	return s.spanContext
}
