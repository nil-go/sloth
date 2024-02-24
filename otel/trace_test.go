// Copyright (c) 2024 The sloth authors
// Use of this source code is governed by a MIT license found in the LICENSE file.

package otel_test

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/nil-go/sloth/otel"
	"github.com/nil-go/sloth/otel/internal/assert"
)

var (
	traceID = [16]byte{75, 249, 47, 53, 119, 179, 77, 166, 163, 206, 146, 157, 14, 14, 71, 54}
	spanID  = [8]byte{0, 240, 103, 170, 11, 169, 2, 183}
)

func TestTraceSampler(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		description string
		traceFlags  byte
		expected    bool
	}{
		{
			description: "trace is not enabled",
			traceFlags:  255,
			expected:    true,
		},
		{
			description: "trace is sampled",
			traceFlags:  1,
			expected:    true,
		},
		{
			description: "trace is not sampled",
			traceFlags:  0,
			expected:    false,
		},
	}

	for _, testcase := range testcases {
		testcase := testcase

		t.Run(testcase.description, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			if testcase.traceFlags != 255 {
				ctx = spanContext(ctx, testcase.traceFlags)
			}

			assert.Equal(t, testcase.expected, otel.TraceSampler(ctx))
		})
	}
}

func spanContext(ctx context.Context, traceFlags byte) context.Context {
	spanContext := trace.SpanContext{}
	spanContext = spanContext.WithTraceID(traceID)
	spanContext = spanContext.WithSpanID(spanID)
	spanContext = spanContext.WithTraceFlags(trace.TraceFlags(traceFlags))

	return trace.ContextWithSpanContext(ctx, spanContext)
}

func TestTraceContext(t *testing.T) {
	t.Parallel()

	ctx := spanContext(context.Background(), 1)
	traceContext := otel.TraceContext(ctx)
	assert.Equal(t, traceID, traceContext.TraceID())
	assert.Equal(t, spanID, traceContext.SpanID())
	assert.Equal(t, 1, traceContext.TraceFlags())
}
