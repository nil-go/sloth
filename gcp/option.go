// Copyright (c) 2024 The sloth authors
// Use of this source code is governed by a MIT license found in the LICENSE file.

package gcp

import (
	"context"
	"io"
	"log/slog"
)

// WithLevel provides the minimum record level that will be logged.
// The handler discards records with lower levels.
//
// If Level is nil, the handler assumes LevelInfo.
func WithLevel(level slog.Leveler) Option {
	return func(options *options) {
		options.level = level
	}
}

// WithWriter provides the writer to which the handler writes.
//
// If Writer is nil, the handler assumes os.Stderr.
func WithWriter(writer io.Writer) Option {
	return func(options *options) {
		options.writer = writer
	}
}

// WithTrace enables [trace information] added to the log for [GCP Cloud Trace] integration.
// The handler use function set in WithTraceContext to get trace information
// if it does not present in record's attributes yet.
//
// [trace information]: https://cloud.google.com/trace/docs/trace-log-integration
// [GCP Cloud Trace]: https://cloud.google.com/trace
func WithTrace(project string) Option {
	return func(options *options) {
		options.project = project
	}
}

// WithTraceContext providers the [W3C Trace Context] while WithTrace has been called.
//
// If it is nil, the handler finds trace information from record's attributes.
//
// [W3C Trace Context]: https://www.w3.org/TR/trace-context/#traceparent-header-field-values
func WithTraceContext(provider func(context.Context) (traceID [16]byte, spanID [8]byte, traceFlags byte)) Option {
	return func(options *options) {
		options.contextProvider = provider
	}
}

// WithErrorReporting enables logs reported as [error events] to [GCP Error Reporting].
//
// [error events]: https://cloud.google.com/error-reporting/docs/formatting-error-messages
// [GCP Error Reporting]: https://cloud.google.com/error-reporting
func WithErrorReporting(service, version string) Option {
	return func(options *options) {
		options.service = service
		options.version = version
	}
}

// WithCallers provides a function to get callers on the calling goroutine's stack
// while WithErrorReporting has been called.
// If the callers returns empty slice, the handler gets stack trace from debug.Stack.
//
// If Callers is nil, the handler checks method `Callers() []uintptr` on the error.
func WithCallers(callers func(error) []uintptr) Option {
	return func(options *options) {
		options.callers = callers
	}
}

type (
	// Option configures the Handler with specific options.
	Option  func(*options)
	options struct {
		writer io.Writer
		level  slog.Leveler

		// For trace.
		project         string
		contextProvider func(context.Context) (traceID [16]byte, spanID [8]byte, traceFlags byte)

		// For error reporting.
		service string
		version string
		callers func(error) []uintptr
	}
)
