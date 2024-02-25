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
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Keys for [W3C Trace Context] attributes.
//
// [W3C Trace Context]: https://www.w3.org/TR/trace-context/#traceparent-header-field-values
const (
	// TraceKey is the key used by the [ID of the whole trace] forest and is used to uniquely
	// identify a distributed trace through a system. It is represented as a 16-byte array,
	// for example, 4bf92f3577b34da6a3ce929d0e0e4736.
	// All bytes as zero (00000000000000000000000000000000) is considered an invalid value.
	//
	// [ID of the whole trace]: https://www.w3.org/TR/trace-context/#trace-id
	TraceKey = "trace-id"
	// SpanKey is the key used by the [ID of this request] as known by the caller.
	// It is represented as an 8-byte array, for example, 00f067aa0ba902b7.
	// All bytes as zero (0000000000000000) is considered an invalid value.
	//
	// [ID of this request]: https://www.w3.org/TR/trace-context/#parent-id
	SpanKey = "span-id"
	// TraceFlagsKey is the key used by an 8-bit field that controls [tracing flags]
	// such as sampling, trace level, etc.
	//
	// [tracing flags]: https://www.w3.org/TR/trace-context/#trace-flags
	TraceFlagsKey = "trace-flags"
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

type eventHandler struct {
	prefix string
	attrs  []attribute.KeyValue
}

func (e eventHandler) Enabled(ctx context.Context) bool {
	span := trace.SpanFromContext(ctx)

	return span.IsRecording() && span.SpanContext().IsSampled()
}

func (e eventHandler) Handle(ctx context.Context, record slog.Record) {
	attrs := slices.Clone(e.attrs)
	attrs = slices.Grow(attrs, record.NumAttrs())
	errs := make(map[string]error)
	record.Attrs(
		func(attr slog.Attr) bool {
			if err, ok := attr.Value.Resolve().Any().(error); ok {
				errs[attr.Key] = err
			} else {
				attrs = append(attrs, convertAttr(attr, e.prefix)...)
			}

			return true
		},
	)

	span := trace.SpanFromContext(ctx)
	switch {
	case record.Level >= slog.LevelError:
		span.SetStatus(codes.Error, record.Message)

		var err error
		for _, e := range errs {
			err = errors.Join(err, e)
		}
		if err == nil {
			err = errors.New(record.Message) //nolint:goerr113
		} else {
			err = fmt.Errorf("%s: %w", record.Message, err)
		}
		span.RecordError(err, trace.WithTimestamp(record.Time), trace.WithAttributes(attrs...), trace.WithStackTrace(true))
	default:
		for k, v := range errs {
			attrs = append(attrs, attribute.String(e.prefix+k, v.Error()))
		}
		span.AddEvent(record.Message, trace.WithTimestamp(record.Time), trace.WithAttributes(attrs...))
	}
}

func (e eventHandler) WithAttrs(attrs []slog.Attr) eventHandler {
	e.attrs = slices.Clone(e.attrs)
	for _, attr := range attrs {
		e.attrs = append(e.attrs, convertAttr(attr, e.prefix)...)
	}

	return e
}

func (e eventHandler) WithGroup(name string) eventHandler {
	e.prefix = e.prefix + name + "."

	return e
}

func convertAttr(attr slog.Attr, prefix string) []attribute.KeyValue { //nolint:cyclop,funlen
	key := prefix + attr.Key
	value := attr.Value

	attrs := make([]attribute.KeyValue, 0, 1)
	switch value.Kind() {
	case slog.KindAny:
		switch val := value.Any().(type) {
		case []string:
			attrs = append(attrs, attribute.StringSlice(key, val))
		case []int:
			attrs = append(attrs, attribute.IntSlice(key, val))
		case []int64:
			attrs = append(attrs, attribute.Int64Slice(key, val))
		case []float64:
			attrs = append(attrs, attribute.Float64Slice(key, val))
		case []bool:
			attrs = append(attrs, attribute.BoolSlice(key, val))
		case fmt.Stringer:
			attrs = append(attrs, attribute.Stringer(key, val))
		default:
			attrs = append(attrs, attribute.String(key, fmt.Sprintf("%v", val)))
		}
	case slog.KindBool:
		attrs = append(attrs, attribute.Bool(key, value.Bool()))
	case slog.KindDuration:
		attrs = append(attrs, attribute.String(key, value.Duration().String()))
	case slog.KindFloat64:
		attrs = append(attrs, attribute.Float64(key, value.Float64()))
	case slog.KindInt64:
		attrs = append(attrs, attribute.Int64(key, value.Int64()))
	case slog.KindString:
		attrs = append(attrs, attribute.String(key, value.String()))
	case slog.KindTime:
		attrs = append(attrs, attribute.String(key, value.Time().Format(time.RFC3339Nano)))
	case slog.KindUint64:
		attrs = append(attrs, attribute.String(key, strconv.FormatUint(value.Uint64(), 10)))
	case slog.KindGroup:
		attrs = slices.Grow(attrs, len(value.Group()))
		for _, groupAttr := range value.Group() {
			attrs = append(attrs, convertAttr(groupAttr, key+".")...)
		}
	case slog.KindLogValuer:
		attr.Value = attr.Value.Resolve()
		attrs = append(attrs, convertAttr(attr, prefix)...)
	}

	return attrs
}
