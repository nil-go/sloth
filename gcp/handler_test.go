// Copyright (c) 2024 The sloth authors
// Use of this source code is governed by a MIT license found in the LICENSE file.

package gcp_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nil-go/sloth/gcp"
	"github.com/nil-go/sloth/internal/assert"
)

func TestHandler(t *testing.T) {
	t.Parallel()

	for _, testcase := range testcases() {
		testcase := testcase

		t.Run(testcase.description, func(t *testing.T) {
			t.Parallel()

			buf := &bytes.Buffer{}
			handler := gcp.New(append(testcase.opts, gcp.WithWriter(buf))...)

			ctx := context.Background()
			if handler.Enabled(ctx, slog.LevelInfo) {
				attrs := []slog.Attr{
					slog.String("a", "A"),
					slog.String("trace_id", "4bf92f3577b34da6a3ce929d0e0e4736"),
					slog.String("span_id", "00f067aa0ba902b7"),
					slog.String("trace_flags", "01"),
				}
				assert.NoError(t, handler.WithAttrs(attrs).Handle(ctx, record(slog.LevelInfo, "info")))
			}
			gHandler := handler.WithGroup("g")
			if handler.Enabled(ctx, slog.LevelWarn) {
				assert.NoError(t, gHandler.WithAttrs([]slog.Attr{slog.String("b", "B")}).
					Handle(ctx, record(slog.LevelWarn, "warn", "a", "A")))
			}
			if handler.Enabled(ctx, slog.LevelError) {
				var attrs []any
				if testcase.err != nil {
					attrs = append(attrs, "error", testcase.err)
				}
				assert.NoError(t, gHandler.WithGroup("h").WithAttrs([]slog.Attr{slog.String("b", "B")}).
					Handle(ctx, record(slog.LevelError, "error", attrs...)))
			}

			path, err := os.Getwd()
			assert.NoError(t, err)
			log, after, _ := strings.Cut(buf.String(), "goroutine ")
			_, after, _ = strings.Cut(after, "[running]:")
			before, after, _ := strings.Cut(after, " +0x")
			_, after, _ = strings.Cut(after, ",")
			log = strings.ReplaceAll(log+before+after, path, "")
			assert.Equal(t, testcase.expected, log)
		})
	}
}

type stackError struct {
	error
}

func (stackError) Callers() []uintptr {
	var pcs [1]uintptr
	runtime.Callers(1, pcs[:])

	return pcs[:]
}

func record(level slog.Level, message string, attrs ...any) slog.Record {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:])

	record := slog.NewRecord(time.Unix(100, 1000), level, message, pcs[0])
	record.Add(attrs...)

	return record
}

//nolint:lll
func testcases() []struct {
	description string
	opts        []gcp.Option
	err         error
	expected    string
} {
	return []struct {
		description string
		opts        []gcp.Option
		err         error
		expected    string
	}{
		{
			description: "default",
			expected: `{"timestamp":{"seconds":100,"nanos":1000},"severity":"INFO","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":41},"message":"info","a":"A","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7","trace_flags":"01"}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":46},"message":"warn","g":{"b":"B","a":"A"}}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"ERROR","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":54},"message":"error","g":{"h":{"b":"B"}}}
`,
		},
		{
			description: "with level",
			opts: []gcp.Option{
				gcp.WithLevel(slog.LevelWarn),
			},
			expected: `{"timestamp":{"seconds":100,"nanos":1000},"severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":46},"message":"warn","g":{"b":"B","a":"A"}}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"ERROR","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":54},"message":"error","g":{"h":{"b":"B"}}}
`,
		},
		{
			description: "with error reporting (original stack)",
			opts: []gcp.Option{
				gcp.WithErrorReporting("test", "dev"),
			},
			err: errors.New("an error"),
			expected: `{"timestamp":{"seconds":100,"nanos":1000},"severity":"INFO","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":41},"message":"info","a":"A","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7","trace_flags":"01"}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":46},"message":"warn","g":{"b":"B","a":"A"}}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"ERROR","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":54},"message":"error","context":{"reportLocation":{"filePath":"/handler_test.go","lineNumber":54,"functionName":"github.com/nil-go/sloth/gcp_test.TestHandler.func1"}},"serviceContext":{"service":"test","version":"dev"},"stack_trace":"error\n\n\ngithub.com/nil-go/sloth/gcp_test.TestHandler.func1()\n\t/handler_test.go:54"g":{"h":{"b":"B","error":"an error"}}}
`,
		},
		{
			description: "with error reporting (caller stack)",
			opts: []gcp.Option{
				gcp.WithErrorReporting("test", "dev"),
				gcp.WithCallers(func(error) []uintptr {
					var pcs [1]uintptr
					runtime.Callers(1, pcs[:])

					return pcs[:]
				}),
			},
			err: errors.New("an error"),
			expected: `{"timestamp":{"seconds":100,"nanos":1000},"severity":"INFO","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":41},"message":"info","a":"A","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7","trace_flags":"01"}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":46},"message":"warn","g":{"b":"B","a":"A"}}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"ERROR","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":54},"message":"error","context":{"reportLocation":{"filePath":"/handler_test.go","lineNumber":54,"functionName":"github.com/nil-go/sloth/gcp_test.TestHandler.func1"}},"serviceContext":{"service":"test","version":"dev"},"stack_trace":"error\n\n\ngithub.com/nil-go/sloth/gcp_test.testcases.func1()\n\t/handler_test.go:136"g":{"h":{"b":"B","error":"an error"}}}
`,
		},
		{
			description: "with error reporting (error stack)",
			opts: []gcp.Option{
				gcp.WithErrorReporting("test", "dev"),
			},
			err: stackError{errors.New("an error")},
			expected: `{"timestamp":{"seconds":100,"nanos":1000},"severity":"INFO","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":41},"message":"info","a":"A","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7","trace_flags":"01"}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":46},"message":"warn","g":{"b":"B","a":"A"}}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"ERROR","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":54},"message":"error","context":{"reportLocation":{"filePath":"/handler_test.go","lineNumber":54,"functionName":"github.com/nil-go/sloth/gcp_test.TestHandler.func1"}},"serviceContext":{"service":"test","version":"dev"},"stack_trace":"error\n\n\ngithub.com/nil-go/sloth/gcp_test.stackError.Callers()\n\t/handler_test.go:75"g":{"h":{"b":"B","error":"an error"}}}
`,
		},
		{
			description: "with trace",
			opts: []gcp.Option{
				gcp.WithTrace("test"),
			},
			expected: `{"timestamp":{"seconds":100,"nanos":1000},"severity":"INFO","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":41},"message":"info","a":"A","logging.googleapis.com/trace":"projects/test/traces/4bf92f3577b34da6a3ce929d0e0e4736","logging.googleapis.com/spanId":"00f067aa0ba902b7","logging.googleapis.com/trace_sampled":true}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":46},"message":"warn","g":{"b":"B","a":"A"}}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"ERROR","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":54},"message":"error","g":{"h":{"b":"B"}}}
`,
		},
		{
			description: "with trace context",
			opts: []gcp.Option{
				gcp.WithTrace("test"),
				gcp.WithTraceContext(func(context.Context) ([16]byte, [8]byte, byte) {
					return [16]byte{75, 249, 47, 53, 119, 179, 77, 166, 163, 206, 146, 157, 14, 14, 71, 54},
						[8]byte{0, 240, 103, 170, 11, 169, 2, 183},
						1
				}),
			},
			expected: `{"timestamp":{"seconds":100,"nanos":1000},"severity":"INFO","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":41},"message":"info","a":"A","logging.googleapis.com/trace":"projects/test/traces/4bf92f3577b34da6a3ce929d0e0e4736","logging.googleapis.com/spanId":"00f067aa0ba902b7","logging.googleapis.com/trace_sampled":true}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":46},"message":"warn","logging.googleapis.com/trace":"projects/test/traces/4bf92f3577b34da6a3ce929d0e0e4736","logging.googleapis.com/spanId":"00f067aa0ba902b7","logging.googleapis.com/trace_sampled":true,"g":{"b":"B","a":"A"}}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"ERROR","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func1","file":"/handler_test.go","line":54},"message":"error","logging.googleapis.com/trace":"projects/test/traces/4bf92f3577b34da6a3ce929d0e0e4736","logging.googleapis.com/spanId":"00f067aa0ba902b7","logging.googleapis.com/trace_sampled":true,"g":{"h":{"b":"B"}}}
`,
		},
	}
}
