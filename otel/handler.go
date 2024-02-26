// Copyright (c) 2024 The sloth authors
// Use of this source code is governed by a MIT license found in the LICENSE file.

/*
Package otel provides a handler for correlation between log records and Open Telemetry spans.

It adds [W3C Trace Context] attributes to log records if there is a span in the context,
so the logs could be correlated with the spans in the distributed tracing system.

It also records log records as trace span's events if it's enabled.
*/
package otel

import (
	"context"
	"encoding/hex"
	"log/slog"
	"slices"

	"go.opentelemetry.io/otel/trace"
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

// Handler correlates log records with Open Telemetry spans.
//
// To create a new Handler, call [New].
type Handler struct {
	handler slog.Handler

	recordEvent bool
	passThrough bool

	groups       []group
	eventHandler eventHandler
}

type group struct {
	name  string
	attrs []slog.Attr
}

// New creates a new Handler with the given Option(s).
func New(handler slog.Handler, opts ...Option) Handler {
	if handler == nil {
		panic("cannot create Handler with nil handler")
	}

	option := &options{handler: handler, eventHandler: eventHandler{}}
	for _, opt := range opts {
		opt(option)
	}

	return Handler(*option)
}

func (h Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h Handler) Handle(ctx context.Context, record slog.Record) error {
	var attrs []slog.Attr

	if spanContext := trace.SpanContextFromContext(ctx); spanContext.IsValid() {
		tid := spanContext.TraceID()
		sid := spanContext.SpanID()
		flags := spanContext.TraceFlags()
		attrs = append(attrs,
			slog.String(TraceKey, hex.EncodeToString(tid[:])),
			slog.String(SpanKey, hex.EncodeToString(sid[:])),
			slog.String(TraceFlagsKey, hex.EncodeToString([]byte{byte(flags)})),
		)
	}

	if h.recordEvent && h.eventHandler.Enabled(ctx) {
		h.eventHandler.Handle(ctx, record)
		if !h.passThrough {
			return nil
		}
	}

	// Have to add the attributes to the handler before adding the group.
	// Otherwise, the attributes are added to the group.
	handler := h.handler.WithAttrs(attrs)
	for _, group := range h.groups {
		handler = handler.WithGroup(group.name).WithAttrs(group.attrs)
	}

	return handler.Handle(ctx, record)
}

func (h Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.eventHandler = h.eventHandler.WithAttrs(attrs)

	if len(h.groups) == 0 {
		h.handler = h.handler.WithAttrs(attrs)

		return h
	}
	h.groups = slices.Clone(h.groups)
	h.groups[len(h.groups)-1].attrs = slices.Clone(h.groups[len(h.groups)-1].attrs)
	h.groups[len(h.groups)-1].attrs = append(h.groups[len(h.groups)-1].attrs, attrs...)

	return h
}

func (h Handler) WithGroup(name string) slog.Handler {
	h.eventHandler = h.eventHandler.WithGroup(name)

	h.groups = slices.Clone(h.groups)
	h.groups = append(h.groups, group{name: name})

	return h
}
