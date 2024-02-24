// Copyright (c) 2024 The sloth authors
// Use of this source code is governed by a MIT license found in the LICENSE file.

/*
Package gcp provides a handler for emitting log records to [GCP Cloud Logging].

The handler formats records to match [GCP Cloud Logging JSON schema].
It also integrates logs with [GCP Cloud Trace] and [GCP Error Reporting] if enabled.

[GCP Cloud Logging]: https://cloud.google.com/logging
[GCP Cloud Logging JSON schema]: https://cloud.google.com/logging/docs/agent/logging/configuration#special-fields
[GCP Cloud Trace]: https://cloud.google.com/trace
[GCP Error Reporting]: https://cloud.google.com/error-reporting
*/
package gcp

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
)

// New creates a new Handler with the given Option(s).
// The handler formats records to match [GCP Cloud Logging JSON schema].
//
// [GCP Cloud Logging JSON schema]: https://cloud.google.com/logging/docs/agent/logging/configuration#special-fields
func New(opts ...Option) slog.Handler {
	option := &options{}
	for _, opt := range opts {
		opt(option)
	}
	if option.writer == nil {
		option.writer = os.Stderr
	}

	var handler slog.Handler
	handler = slog.NewJSONHandler(
		option.writer,
		&slog.HandlerOptions{
			AddSource:   true,
			Level:       option.level,
			ReplaceAttr: replaceAttr,
		},
	)
	if option.project != "" || option.service != "" {
		if option.callers == nil {
			option.callers = func(err error) []uintptr {
				var callers interface{ Callers() []uintptr }
				if errors.As(err, &callers) {
					return callers.Callers()
				}

				return nil
			}
		}

		handler = logHandler{
			handler: handler,
			project: option.project, contextProvider: option.contextProvider,
			service: option.service, version: option.version,
			callers: option.callers,
		}
	}

	return handler
}

func replaceAttr(groups []string, attr slog.Attr) slog.Attr { //nolint:cyclop
	if len(groups) > 0 {
		return attr
	}

	// Replace attributes to match GCP Cloud Logging format.
	//
	// See: https://cloud.google.com/logging/docs/agent/logging/configuration#special-fields
	switch attr.Key {
	// Maps the slog levels to the correct [severity] for GCP Cloud Logging.
	//
	// See: https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity
	case slog.LevelKey:
		var severity string
		if level, ok := attr.Value.Any().(slog.Level); ok {
			switch {
			case level >= slog.LevelError:
				severity = "ERROR"
			case level >= slog.LevelWarn:
				severity = "WARNING"
			case level >= slog.LevelInfo:
				severity = "INFO"
			default:
				severity = "DEBUG"
			}
		}

		return slog.String("severity", severity)

	// Format event timestamp according to GCP JSON formats.
	//
	// See: https://cloud.google.com/logging/docs/agent/logging/configuration#timestamp-processing
	case slog.TimeKey:
		time := attr.Value.Time()

		return slog.Attr{
			Key: "timestamp",
			Value: slog.GroupValue(
				slog.Int64("seconds", time.Unix()),
				slog.Int64("nanos", int64(time.Nanosecond())),
			),
		}

	case slog.SourceKey:
		attr.Key = "logging.googleapis.com/sourceLocation"

		return attr

	case slog.MessageKey:
		attr.Key = "message"

		return attr

	default:
		return attr
	}
}

type (
	logHandler struct {
		handler slog.Handler
		groups  []group

		project         string
		contextProvider func(context.Context) TraceContext

		service string
		version string
		callers func(error) []uintptr
	}
	group struct {
		name  string
		attrs []slog.Attr
	}
)

func (h logHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h logHandler) Handle(ctx context.Context, record slog.Record) error { //nolint:cyclop,funlen
	handler := h.handler

	if len(h.groups) > 0 {
		var (
			attr    slog.Attr
			hasAttr bool
		)
		for i := len(h.groups) - 1; i >= 0; i-- {
			grp := h.groups[i]

			attrs := slices.Clone(grp.attrs)
			if hasAttr {
				attrs = append(attrs, attr)
			}

			if len(attrs) > 0 {
				attr = slog.Attr{
					Key:   grp.name,
					Value: slog.GroupValue(attrs...),
				}
				hasAttr = true
			}
		}
		if hasAttr {
			handler = handler.WithAttrs([]slog.Attr{attr})
		}
	}

	// Associate logs with a trace and span.
	//
	// See: https://cloud.google.com/trace/docs/trace-log-integration
	if h.project != "" {
		const sampled = 0x1

		if traceContext := h.contextProvider(ctx); traceContext.TraceID() != [16]byte{} {
			traceID := traceContext.TraceID()
			spanID := traceContext.SpanID()
			traceFlags := traceContext.TraceFlags()
			handler = handler.WithAttrs([]slog.Attr{
				slog.String("logging.googleapis.com/trace", "projects/"+h.project+"/traces/"+hex.EncodeToString(traceID[:])),
				slog.String("logging.googleapis.com/spanId", hex.EncodeToString(spanID[:])),
				slog.Bool("logging.googleapis.com/trace_sampled", traceFlags&sampled == sampled),
			})
		}
	}

	// Format log to report error events.
	//
	// See: https://cloud.google.com/error-reporting/docs/formatting-error-messages
	if record.Level >= slog.LevelError && h.service != "" {
		firstFrame, _ := runtime.CallersFrames([]uintptr{record.PC}).Next()
		var stackTrace strings.Builder
		stackTrace.WriteString(record.Message)
		stackTrace.WriteString("\n\n")

		var callers []uintptr
		record.Attrs(func(attr slog.Attr) bool {
			if err, ok := attr.Value.Any().(error); ok {
				callers = h.callers(err)

				return false
			}

			return true
		})

		if len(callers) > 0 {
			stackFromCallers(&stackTrace, callers)
		} else {
			stack(&stackTrace, firstFrame)
		}

		handler = handler.WithAttrs(
			[]slog.Attr{
				{
					Key: "context",
					Value: slog.GroupValue(
						slog.Attr{
							Key: "reportLocation",
							Value: slog.GroupValue(
								slog.String("filePath", firstFrame.File),
								slog.Int("lineNumber", firstFrame.Line),
								slog.String("functionName", firstFrame.Function),
							),
						},
					),
				},
				{
					Key: "serviceContext",
					Value: slog.GroupValue(
						slog.String("service", h.service),
						slog.String("version", h.version),
					),
				},
				slog.String("stack_trace", stackTrace.String()),
			})
	}

	for _, group := range h.groups {
		handler = handler.WithGroup(group.name)
	}

	return handler.Handle(ctx, record)
}

func stackFromCallers(stackTrace *strings.Builder, callers []uintptr) {
	stackTrace.WriteString("goroutine 1 [running]:\n")
	frames := runtime.CallersFrames(callers)
	for {
		frame, more := frames.Next()
		stackTrace.WriteString(frame.Function)
		stackTrace.WriteString("()\n\t")
		stackTrace.WriteString(frame.File)
		stackTrace.WriteString(":")
		stackTrace.WriteString(strconv.Itoa(frame.Line))
		stackTrace.WriteString(" +0x")
		stackTrace.WriteString(strconv.FormatUint(uint64(frame.PC-frame.Entry), 16))
		stackTrace.WriteString("\n")

		if !more {
			break
		}
	}
}

func stack(stackTrace *strings.Builder, firstFrame runtime.Frame) {
	frames := bytes.NewBuffer(debug.Stack())
	// Always add the first line (goroutine line) in stack trace.
	firstLine, err := frames.ReadBytes('\n')
	stackTrace.Write(firstLine)
	if err != nil {
		return
	}

	// Each frame has 2 lines in stack trace, first line is the function and second line is the file:#line.
	firstFuncLine := []byte(firstFrame.Function)
	var functionLine, fileLine []byte
	for {
		if functionLine, err = frames.ReadBytes('\n'); err != nil {
			break
		}
		if fileLine, err = frames.ReadBytes('\n'); err != nil {
			break
		}
		if bytes.Contains(functionLine, firstFuncLine) {
			stackTrace.Write(functionLine)
			stackTrace.Write(fileLine)
			stackTrace.Write(frames.Bytes())

			break
		}
	}
}

func (h logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(h.groups) == 0 {
		h.handler = h.handler.WithAttrs(attrs)

		return h
	}

	h.groups = slices.Clone(h.groups)
	h.groups[len(h.groups)-1].attrs = slices.Clone(h.groups[len(h.groups)-1].attrs)
	h.groups[len(h.groups)-1].attrs = append(h.groups[len(h.groups)-1].attrs, attrs...)

	return h
}

func (h logHandler) WithGroup(name string) slog.Handler {
	h.groups = slices.Clone(h.groups)
	h.groups = append(h.groups, group{name: name})

	return h
}
