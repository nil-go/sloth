// Copyright (c) 2024 The sloth authors
// Use of this source code is governed by a MIT license found in the LICENSE file.

package gcp_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nil-go/sloth/gcp"
	"github.com/nil-go/sloth/internal/assert"
)

//nolint:lll
func TestHandler(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		description string
		opts        []gcp.Option
		expected    string
	}{
		{
			description: "default",
			expected: `{"timestamp":{"seconds":100,"nanos":1000},"severity":"INFO","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func2","file":"/handler_test.go","line":79},"message":"info","a":"A"}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func2","file":"/handler_test.go","line":83},"message":"warn","g":{"a":"A"}}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"ERROR","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func2","file":"/handler_test.go","line":89},"message":"error","g":{"h":{"b":"B"}}}
`,
		},
		{
			description: "with level",
			opts: []gcp.Option{
				gcp.WithLevel(slog.LevelWarn),
			},
			expected: `{"timestamp":{"seconds":100,"nanos":1000},"severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func2","file":"/handler_test.go","line":83},"message":"warn","g":{"a":"A"}}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"ERROR","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func2","file":"/handler_test.go","line":89},"message":"error","g":{"h":{"b":"B"}}}
`,
		},
		{
			description: "with error reporting",
			opts: []gcp.Option{
				gcp.WithErrorReporting("test", "dev"),
			},
			expected: `{"timestamp":{"seconds":100,"nanos":1000},"severity":"INFO","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func2","file":"/handler_test.go","line":79},"message":"info","a":"A"}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func2","file":"/handler_test.go","line":83},"message":"warn","g":{"a":"A"}}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"ERROR","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func2","file":"/handler_test.go","line":89},"message":"error","g":{"h":{"b":"B"}},"context":{"reportLocation":{"filePath":"/handler_test.go","lineNumber":89,"functionName":"github.com/nil-go/sloth/gcp_test.TestHandler.func2"}},"serviceContext":{"service":"test","version":"dev"},"stack_trace":"error\n\n`,
		},
		{
			description: "with trace",
			opts: []gcp.Option{
				gcp.WithTrace("test", func(context.Context) gcp.TraceContext { return traceContext{} }),
			},
			expected: `{"timestamp":{"seconds":100,"nanos":1000},"severity":"INFO","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func2","file":"/handler_test.go","line":79},"message":"info","a":"A","logging.googleapis.com/trace":"projects/test/traces/4bf92f3577b34da6a3ce929d0e0e4736","logging.googleapis.com/spanId":"00f067aa0ba902b7","logging.googleapis.com/trace_sampled":true}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func2","file":"/handler_test.go","line":83},"message":"warn","logging.googleapis.com/trace":"projects/test/traces/4bf92f3577b34da6a3ce929d0e0e4736","logging.googleapis.com/spanId":"00f067aa0ba902b7","logging.googleapis.com/trace_sampled":true,"g":{"a":"A"}}
{"timestamp":{"seconds":100,"nanos":1000},"severity":"ERROR","logging.googleapis.com/sourceLocation":{"function":"github.com/nil-go/sloth/gcp_test.TestHandler.func2","file":"/handler_test.go","line":89},"message":"error","g":{"h":{"b":"B"}},"logging.googleapis.com/trace":"projects/test/traces/4bf92f3577b34da6a3ce929d0e0e4736","logging.googleapis.com/spanId":"00f067aa0ba902b7","logging.googleapis.com/trace_sampled":true}
`,
		},
	}

	for _, testcase := range testcases {
		testcase := testcase

		t.Run(testcase.description, func(t *testing.T) {
			t.Parallel()

			buf := &bytes.Buffer{}
			handler := gcp.New(append(testcase.opts, gcp.WithWriter(buf))...)

			ctx := context.Background()
			if handler.Enabled(ctx, slog.LevelInfo) {
				assert.NoError(t, handler.WithAttrs([]slog.Attr{slog.String("a", "A")}).
					Handle(ctx, record(slog.LevelInfo, "info")))
			}
			gHandler := handler.WithGroup("g")
			if handler.Enabled(ctx, slog.LevelWarn) {
				record := record(slog.LevelWarn, "warn")
				record.Add("a", "A")
				assert.NoError(t, gHandler.Handle(ctx, record))
			}
			if handler.Enabled(ctx, slog.LevelError) {
				assert.NoError(t, gHandler.WithGroup("h").WithAttrs([]slog.Attr{slog.String("b", "B")}).
					Handle(ctx, record(slog.LevelError, "error")))
			}

			path, err := os.Getwd()
			assert.NoError(t, err)
			log, _, _ := strings.Cut(buf.String(), "goroutine ")
			log = strings.ReplaceAll(log, path, "")
			assert.Equal(t, testcase.expected, log)
		})
	}
}

func record(level slog.Level, message string) slog.Record {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:])

	return slog.NewRecord(time.Unix(100, 1000), level, message, pcs[0])
}

type traceContext struct{}

func (traceContext) TraceID() [16]byte {
	b, _ := hex.DecodeString("4bf92f3577b34da6a3ce929d0e0e4736")

	return [16]byte(b)
}

func (traceContext) SpanID() [8]byte {
	b, _ := hex.DecodeString("00f067aa0ba902b7")

	return [8]byte(b)
}

func (traceContext) TraceFlags() byte {
	return 1
}
