// Copyright (c) 2024 The sloth authors
// Use of this source code is governed by a MIT license found in the LICENSE file.

package otel

// WithRecordEvent enables recording log records as trace span's events.
// If passThrough is true, the log record will pass through to the next handler.
//
// If the level is less than slog.LevelError, the log record will be recorded as an event.
// Otherwise. the log record will be recorded as an exception event and set the status of span to Error.
func WithRecordEvent(passThrough bool) Option {
	return func(options *options) {
		options.recordEvent = true
		options.passThrough = passThrough
	}
}

type (
	// Option configures the Handler with specific options.
	Option  func(*options)
	options Handler
)
