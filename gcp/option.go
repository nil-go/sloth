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
//
// [trace information]: https://cloud.google.com/trace/docs/trace-log-integration
// [GCP Cloud Trace]: https://cloud.google.com/trace
func WithTrace(project string, contextProvider func(context.Context) TraceContext) Option {
	if project == "" {
		panic("cannot add trace information with empty project")
	}
	if contextProvider == nil {
		panic("cannot add trace information with nil context provider")
	}

	return func(options *options) {
		options.project = project
		options.contextProvider = contextProvider
	}
}

// TraceContext providers the [W3C Trace Context].
//
// [W3C Trace Context]: https://www.w3.org/TR/trace-context/#trace-id
type TraceContext interface {
	TraceID() [16]byte
	SpanID() [8]byte
	TraceFlags() byte
}

// WithErrorReporting enables logs reported as [error events] to [GCP Error Reporting].
//
// [error events]: https://cloud.google.com/error-reporting/docs/formatting-error-messages
// [GCP Error Reporting]: https://cloud.google.com/error-reporting
func WithErrorReporting(service, version string) Option {
	if service == "" {
		panic("cannot add error information with empty service")
	}

	return func(options *options) {
		options.service = service
		options.version = version
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
		contextProvider func(context.Context) TraceContext

		// For error reporting.
		service string
		version string
	}
)
