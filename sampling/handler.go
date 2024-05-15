// Copyright (c) 2024 The sloth authors
// Use of this source code is governed by a MIT license found in the LICENSE file.

/*
Package sampling provides a handler for sampling records at request level.

It discards records with lower than the minimum level if request is unsampled. For example,
if the minimum level is slog.LevelError, it logs records with slog.LevelError and above regardless,
but discards records with slog.LevelWarn and below unless the request is sampled.

It's ok to discard records with lower level if everything is fine. However,
if a record with slog.LevelError logs, it's better to log records with slog.LevelWarn and below
around it so developers could have a context for debugging even the request is not sampled.
To achieve this, Handler.WithBuffer should be called at the beginning interceptor of the gRPC/HTTP request.

	ctx, cancel := h.WithBuffer(ctx)
	defer cancel()
*/
package sampling

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
)

// Handler samples records according to the give sampler.
//
// To create a new Handler, call [New].
type Handler struct {
	handler slog.Handler
	sampler func(ctx context.Context) bool

	level slog.Level
}

type contextKey struct{}

// New creates a new Handler with the given Option(s).
func New(handler slog.Handler, sampler func(ctx context.Context) bool, opts ...Option) Handler {
	if handler == nil {
		panic("cannot create Handler with nil handler")
	}
	if sampler == nil {
		panic("cannot create Handler with nil sampler")
	}

	option := &options{
		handler: handler,
		sampler: sampler,
		level:   slog.LevelError,
	}
	for _, opt := range opts {
		opt(option)
	}

	return Handler(*option)
}

func (h Handler) Enabled(ctx context.Context, level slog.Level) bool {
	if enabled := h.handler.Enabled(ctx, level); !enabled {
		return false
	}

	// If the log has not been sampled and there is no buffer in context,
	// then it only logs while the level is greater than or equal to the handler level.
	if ctx.Value(contextKey{}) == nil && !h.sampler(ctx) {
		return level >= h.level
	}

	return true
}

func (h Handler) Handle(ctx context.Context, record slog.Record) error {
	if h.sampler(ctx) {
		return h.handler.Handle(ctx, record)
	}

	// If there is buffer in context and the log has not been sampled,
	// then the record is handled by the buffer.
	if b, ok := ctx.Value(contextKey{}).(*buffer); ok {
		if record.Level < h.level {
			return b.buffer(ctx, h.handler, record)
		}

		b.drain()
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

// WithBuffer enables log buffering for the request associated with the given context.
// It usually should be called at the beginning interceptor of the gRPC/HTTP request.
//
// Canceling this context releases buffer associated with it, so code should
// call cancel as soon as the operations running in this [Context] complete:
//
//	ctx, cancel := h.WithBuffer(ctx)
//	defer cancel()
func WithBuffer(ctx context.Context) (context.Context, func()) {
	buf := bufferPool.Get().(*buffer) //nolint:forcetypeassert,errcheck
	ctx = context.WithValue(ctx, contextKey{}, buf)

	return ctx, buf.reset
}

type (
	buffer struct {
		entries  chan entry
		overflow []entry
		drained  atomic.Bool
	}

	entry struct {
		handler slog.Handler
		ctx     context.Context //nolint:containedctx
		record  slog.Record
	}
)

func (b *buffer) buffer(ctx context.Context, handler slog.Handler, record slog.Record) error {
	if drained := b.drained.Load(); drained {
		return handler.Handle(ctx, record)
	}

	for {
		select {
		case b.entries <- entry{handler: handler, ctx: ctx, record: record}:
			return nil
		default:
			// If the buffer is full, then move it to overflow.
			if len(b.overflow) == cap(b.overflow) {
				b.overflow = slices.Grow(b.overflow, len(b.entries))
			}
			b.overflow = append(b.overflow, <-b.entries)
		}
	}
}

func (b *buffer) drain() {
	if drained := b.drained.Swap(true); drained {
		return
	}

	for _, e := range b.overflow {
		// Here ignores the error for best effort.
		_ = e.handler.Handle(e.ctx, e.record)
	}
	clear(b.overflow)
	b.overflow = b.overflow[:0]

	for {
		select {
		case e := <-b.entries:
			// Here ignores the error for best effort.
			_ = e.handler.Handle(e.ctx, e.record)
		default:
			return
		}
	}
}

func (b *buffer) reset() {
	if drained := b.drained.Swap(false); !drained {
		// Discard the buffer.
	loop:
		for {
			select {
			case <-b.entries:
			default:
				break loop
			}
		}
	}
	clear(b.overflow)
	b.overflow = b.overflow[:0]

	bufferPool.Put(b)
}

var bufferPool = sync.Pool{ //nolint:gochecknoglobals
	New: func() interface{} {
		return &buffer{
			entries: make(chan entry, 8), //nolint:mnd
		}
	},
}
