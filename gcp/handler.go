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
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// Keys for [W3C Trace Context] attributes by following [Trace Context in non-OTLP Log Formats].
//
// [W3C Trace Context]: https://www.w3.org/TR/trace-context/#traceparent-header-field-values
// [Trace Context in non-OTLP Log Formats]: https://www.w3.org/TR/trace-context/#trace-id
const (
	// TraceKey is the key used by the [ID of the whole trace] forest and is used to uniquely
	// identify a distributed trace through a system. It is represented as a 16-byte array,
	// for example, 4bf92f3577b34da6a3ce929d0e0e4736.
	// All bytes as zero (00000000000000000000000000000000) is considered an invalid value.
	//
	// [ID of the whole trace]: https://www.w3.org/TR/trace-context/#trace-id
	TraceKey = "trace_id"
	// SpanKey is the key used by the [ID of this request] as known by the caller.
	// It is represented as an 8-byte array, for example, 00f067aa0ba902b7.
	// All bytes as zero (0000000000000000) is considered an invalid value.
	//
	// [ID of this request]: https://www.w3.org/TR/trace-context/#parent-id
	SpanKey = "span_id"
	// TraceFlagsKey is the key used by an 8-bit field that controls [tracing flags]
	// such as sampling, trace level, etc.
	//
	// [tracing flags]: https://www.w3.org/TR/trace-context/#trace-flags
	TraceFlagsKey = "trace_flags"
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
			ReplaceAttr: replaceAttr(option.project),
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
			handler:         handler,
			contextProvider: option.contextProvider,
			service:         option.service, version: option.version, callers: option.callers,
		}
	}

	return handler
}

func replaceAttr(project string) func(groups []string, attr slog.Attr) slog.Attr { //nolint:cyclop,funlen
	return func(groups []string, attr slog.Attr) slog.Attr {
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
			if level, ok := attr.Value.Resolve().Any().(slog.Level); ok {
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
			time := attr.Value.Resolve().Time()

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
		}

		// Associate logs with a trace and span.
		//
		// See: https://cloud.google.com/trace/docs/trace-log-integration
		if project != "" {
			switch attr.Key {
			case TraceKey:
				return slog.String("logging.googleapis.com/trace", "projects/"+project+"/traces/"+attr.Value.Resolve().String())
			case SpanKey:
				attr.Key = "logging.googleapis.com/spanId"

				return attr
			case TraceFlagsKey:
				var sampled bool
				flags, _ := hex.DecodeString(attr.Value.Resolve().String())
				if len(flags) > 0 {
					sampled = flags[0]&0x1 == 0x1 //nolint:mnd
				}

				return slog.Bool("logging.googleapis.com/trace_sampled", sampled)
			}
		}

		return attr
	}
}

type (
	logHandler struct {
		handler slog.Handler

		contextProvider func(context.Context) (traceID [16]byte, spanID [8]byte, traceFlags byte)
		hasTrace        bool

		service string
		version string
		callers func(error) []uintptr

		groups []group
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
	var attrs []slog.Attr

	// Associate logs with a trace and span.
	//
	// See: https://cloud.google.com/trace/docs/trace-log-integration
	if !h.hasTrace && h.contextProvider != nil { //nolint:nestif
		var found bool
		// Only search for trace attributes if there are no groups.
		if len(h.groups) == 0 {
			record.Attrs(func(attr slog.Attr) bool {
				if attr.Key == TraceKey {
					found = true

					return false
				}

				return true
			})
		}

		if !found {
			if traceID, spanID, traceFlags := h.contextProvider(ctx); traceID != [16]byte{} {
				attrs = append(attrs,
					slog.String(TraceKey, hex.EncodeToString(traceID[:])),
					slog.String(SpanKey, hex.EncodeToString(spanID[:])),
					slog.String(TraceFlagsKey, hex.EncodeToString([]byte{traceFlags})),
				)
			}
		}
	}

	// Format log to report error events.
	//
	// See: https://cloud.google.com/error-reporting/docs/formatting-error-messages
	if record.Level >= slog.LevelError && h.service != "" {
		firstFrame, _ := runtime.CallersFrames([]uintptr{record.PC}).Next()
		var callers []uintptr
		record.Attrs(func(attr slog.Attr) bool {
			if err, ok := attr.Value.Resolve().Any().(error); ok {
				callers = h.callers(err)

				return false
			}

			return true
		})

		if len(callers) == 0 {
			callers = loadCallers(firstFrame)
		}

		attrs = append(attrs,
			slog.Attr{
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
			slog.Attr{
				Key: "serviceContext",
				Value: slog.GroupValue(
					slog.String("service", h.service),
					slog.String("version", h.version),
				),
			},
			slog.String("stack_trace", stack(record.Message, callers)),
		)
	}

	// Have to add the attributes to the handler before adding the group.
	// Otherwise, the attributes are added to the group.
	handler := h.handler.WithAttrs(attrs)
	for _, group := range h.groups {
		handler = handler.WithGroup(group.name).WithAttrs(group.attrs)
	}

	return handler.Handle(ctx, record)
}

func loadCallers(firstFrame runtime.Frame) []uintptr {
	var pcs [32]uintptr
	count := runtime.Callers(2, pcs[:]) //nolint:mnd // skip [runtime.Callers, this function]

	// Skip frames before the first frame of the record.
	callers := pcs[:count]
	frames := runtime.CallersFrames(callers)
	for {
		frame, more := frames.Next()
		if frame.Function == firstFrame.Function &&
			frame.File == firstFrame.File &&
			frame.Line == firstFrame.Line {
			break
		}
		callers = callers[1:]
		if !more {
			break
		}
	}

	if len(callers) > 0 {
		return callers
	}

	// If the first frame is not found, all frames prints as stack trace.
	return pcs[:count]
}

func stack(message string, callers []uintptr) string {
	var stackTrace strings.Builder
	stackTrace.Grow(128 * len(callers)) //nolint:mnd // It assumes 128 bytes per frame.

	stackTrace.WriteString(message)
	stackTrace.WriteString("\n\n")
	// Always use 1 as the goroutine number as golang does not prove a way to get the current goroutine number.
	// It's meaningless in stace trace since every log may have different goroutine number.
	// It has to be a goroutine line to match the stack trace format for Error Reporting.
	stackTrace.WriteString("goroutine 1 [running]:\n")

	frames := runtime.CallersFrames(callers)
	for {
		// Each frame has 2 lines in stack trace.
		frame, more := frames.Next()
		// The first line is the function.
		stackTrace.WriteString(frame.Function)
		stackTrace.WriteString("()\n")
		// The second line is the file:line.
		stackTrace.WriteString("\t")
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

	return stackTrace.String()
}

func (h logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(h.groups) == 0 {
		h.handler = h.handler.WithAttrs(attrs)
		if slices.ContainsFunc(attrs, func(attr slog.Attr) bool { return attr.Key == TraceKey }) {
			h.hasTrace = true
		}

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
