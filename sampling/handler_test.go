// Copyright (c) 2024 The sloth authors
// Use of this source code is governed by a MIT license found in the LICENSE file.

package sampling_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/nil-go/sloth/internal/assert"
	"github.com/nil-go/sloth/sampling"
)

func TestNew_panic(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		description string
		handler     slog.Handler
		sampler     func(context.Context) bool
		err         string
	}{
		{
			description: "handler is nil",
			sampler:     func(context.Context) bool { return true },
			err:         "cannot create Handler with nil handler",
		},
		{
			description: "sampler is nil",
			handler:     slog.Default().Handler(),
			err:         "cannot create Handler with nil sampler",
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.description, func(t *testing.T) {
			t.Parallel()

			defer func() {
				assert.Equal(t, testcase.err, recover().(string))
			}()

			sampling.New(testcase.handler, testcase.sampler)
			t.Fail()
		})
	}
}

func TestHandler(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		description string
		sampled     bool
		buffered    bool
		level       slog.Level
		expected    string
	}{
		{
			description: "log is sampled",
			sampled:     true,
			level:       slog.LevelWarn,
			expected: `level=INFO msg=info
level=INFO msg=info2
level=WARN msg=warn test.attr=a
level=INFO msg=info3
`,
		},
		{
			description: "log is not buffered",
			level:       slog.LevelWarn,
			expected: `level=WARN msg=warn test.attr=a
`,
		},
		{
			description: "log has minimum level",
			buffered:    true,
			level:       slog.LevelWarn,
			expected: `level=INFO msg=info
level=INFO msg=info2
level=WARN msg=warn test.attr=a
level=INFO msg=info3
`,
		},
		{
			description: "log has lower level",
			buffered:    true,
			level:       slog.LevelError,
			expected:    "",
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.description, func(t *testing.T) {
			t.Parallel()

			buf := &bytes.Buffer{}
			handler := sampling.New(
				slog.NewTextHandler(buf, &slog.HandlerOptions{
					ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
						if len(groups) == 0 && attr.Key == slog.TimeKey {
							return slog.Attr{}
						}

						return attr
					},
				}),
				func(context.Context) bool { return testcase.sampled },
				sampling.WithLevel(testcase.level),
			)
			logger := slog.New(handler)
			ctx := context.Background()
			if testcase.buffered {
				var put func()
				ctx, put = sampling.WithBuffer(ctx)
				defer put()
			}

			logger.DebugContext(ctx, "debug")
			logger.InfoContext(ctx, "info")
			logger.InfoContext(ctx, "info2")
			logger.WithGroup("test").With("attr", "a").WarnContext(ctx, "warn")
			logger.InfoContext(ctx, "info3")
			assert.Equal(t, testcase.expected, buf.String())
		})
	}
}

func TestHandler_overflow(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := sampling.New(
		slog.NewTextHandler(buf, &slog.HandlerOptions{
			ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
				if len(groups) == 0 && attr.Key == slog.TimeKey {
					return slog.Attr{}
				}

				return attr
			},
		}),
		func(context.Context) bool { return false },
	)
	logger := slog.New(handler)

	ctx, put := sampling.WithBuffer(context.Background())
	defer put()

	logger.InfoContext(ctx, "info")
	logger.InfoContext(ctx, "info2")
	logger.InfoContext(ctx, "info3")
	logger.InfoContext(ctx, "info4")
	logger.InfoContext(ctx, "info5")
	logger.InfoContext(ctx, "info6")
	logger.InfoContext(ctx, "info7")
	logger.InfoContext(ctx, "info8")
	logger.InfoContext(ctx, "info9")
	logger.ErrorContext(ctx, "error")
	logger.InfoContext(ctx, "info10")

	expected := `level=INFO msg=info
level=INFO msg=info2
level=INFO msg=info3
level=INFO msg=info4
level=INFO msg=info5
level=INFO msg=info6
level=INFO msg=info7
level=INFO msg=info8
level=INFO msg=info9
level=ERROR msg=error
level=INFO msg=info10
`
	assert.Equal(t, expected, buf.String())
}
