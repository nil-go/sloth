// Copyright (c) 2024 The sloth authors
// Use of this source code is governed by a MIT license found in the LICENSE file.

/*
Package rate provides a Handler that limits records within the given rate.
It caps the CPU and I/O load of logging while attempting to preserve a representative subset of your logs.

It logs the first N records with a given level and message each interval.
If more records with the same level and message are seen during the same interval,
every Mth message is logged and the rest are dropped.

Keep in mind that the implementation is optimized for speed over absolute precision;
under load, each tick may be slightly over- or under-sampled.
*/
package rate

import (
	"context"
	"log/slog"
	"time"
)

// Handler limits records with give rate, which caps the CPU and I/O load
// of logging while attempting to preserve a representative subset of your logs.
//
// To create a new Handler, call [New].
type Handler struct {
	handler slog.Handler

	interval time.Duration
	first    uint64
	every    uint64

	counts *counters
}

// New creates a new Handler with the given Option(s).
func New(handler slog.Handler, opts ...Option) Handler {
	if handler == nil {
		panic("cannot create Handler with nil handler")
	}

	option := &options{
		handler: handler,
		counts:  &counters{},
		every:   100, //nolint:gomnd
	}
	for _, opt := range opts {
		opt(option)
	}
	if option.interval <= 0 {
		option.interval = time.Second
	}
	if option.first == 0 {
		option.first = 100
	}

	return Handler(*option)
}

func (h Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h Handler) Handle(ctx context.Context, record slog.Record) error {
	count := h.counts.get(record.Level, record.Message)
	n := count.Inc(record.Time, h.interval)
	if n > h.first && (h.every == 0 || (n-h.first)%h.every != 0) {
		return nil
	}

	return h.handler.Handle(ctx, record)
}

func (h Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.handler = h.handler.WithAttrs(attrs)

	return h
}

func (h Handler) WithGroup(name string) slog.Handler {
	h.handler = h.handler.WithGroup(name)

	return h
}
