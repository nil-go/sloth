// Copyright (c) 2024 The sloth authors
// Use of this source code is governed by a MIT license found in the LICENSE file.

package rate

import "time"

// WithFirst provides N that logs the first N records with a given level and message each interval.
//
// If the first N is 0, the handler assumes 100.
func WithFirst(first uint64) Option {
	return func(options *options) {
		options.first = first
	}
}

// WithEvery provides M that logs every Mth record after first N records
// with a given level and message each interval.
// If M is 0, it will drop all log records after the first N in that interval.
//
// The default M is 100.
func WithEvery(every uint64) Option {
	return func(options *options) {
		options.every = every
	}
}

// WithInterval provides the interval for rate limiting.
//
// If the interval is <= 0, the handler assumes 1 second.
func WithInterval(interval time.Duration) Option {
	return func(options *options) {
		options.interval = interval
	}
}

type (
	// Option configures the Handler with specific options.
	Option  func(*options)
	options Handler
)
