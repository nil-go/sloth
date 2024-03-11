// Copyright (c) 2024 The sloth authors
// Use of this source code is governed by a MIT license found in the LICENSE file.

package otel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"slices"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

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

	firstFrame, _ := runtime.CallersFrames([]uintptr{record.PC}).Next()
	attrs = append(attrs,
		semconv.CodeFilepath(firstFrame.File),
		semconv.CodeLineNumber(firstFrame.Line),
		semconv.CodeFunction(firstFrame.Function),
	)

	span := trace.SpanFromContext(ctx)
	switch {
	case record.Level >= slog.LevelError:
		var err error
		for _, e := range errs {
			err = errors.Join(err, e)
		}
		if err == nil {
			err = errors.New(record.Message) //nolint:goerr113
		} else {
			err = fmt.Errorf("%s: %w", record.Message, err)
		}
		span.RecordError(err, trace.WithTimestamp(record.Time), trace.WithAttributes(attrs...))
		span.SetStatus(codes.Error, record.Message)
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
