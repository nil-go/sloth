// Copyright (c) 2024 The sloth authors
// Use of this source code is governed by a MIT license found in the LICENSE file.

package sampling

import "log/slog"

// WithLevel provides the minimum record level that will be logged without sampling.
// It discards unsampled records with lower level unless the buffer is activated by Handler.WithBuffer.
//
// The default minimum record level is  slog.LevelError.
func WithLevel(level slog.Level) Option {
	return func(options *options) {
		options.level = level
	}
}

type (
	// Option configures the Handler with specific options.
	Option  func(*options)
	options Handler
)
