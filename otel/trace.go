// Copyright (c) 2024 The sloth authors
// Use of this source code is governed by a MIT license found in the LICENSE file.

package otel

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// TraceSampler returns a sampler that samples records according to the trace sampling decision.
// It always returns true if trace is not enabled.
func TraceSampler(ctx context.Context) bool {
	sc := trace.SpanContextFromContext(ctx)

	return !sc.IsValid() || sc.IsSampled()
}

// TraceContext returns the open telemetry trace context.
func TraceContext(ctx context.Context) SpanContext {
	return SpanContext{spanContext: trace.SpanContextFromContext(ctx)}
}

type SpanContext struct {
	spanContext trace.SpanContext
}

func (t SpanContext) TraceID() [16]byte {
	return t.spanContext.TraceID()
}

func (t SpanContext) SpanID() [8]byte {
	return t.spanContext.SpanID()
}

func (t SpanContext) TraceFlags() byte {
	return byte(t.spanContext.TraceFlags())
}
