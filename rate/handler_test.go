// Copyright (c) 2024 The sloth authors
// Use of this source code is governed by a MIT license found in the LICENSE file.

package rate_test

import (
	"bytes"
	"context"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nil-go/sloth/internal/assert"
	"github.com/nil-go/sloth/rate"
)

func TestNew_panic(t *testing.T) {
	t.Parallel()

	defer func() {
		assert.Equal(t, "cannot create Handler with nil handler", recover().(string))
	}()

	rate.New(nil)
	t.Fail()
}

func TestHandler(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		description string
		level       slog.Level
		expected    string
	}{
		{
			description: "level error",
			level:       slog.LevelError,
			expected: `level=ERROR msg=msg pos=first
level=ERROR msg=msg pos=second
level=ERROR msg=msg pos=fourth
level=ERROR msg=msg g.pos=after
`,
		},
		{
			description: "level warn",
			level:       slog.LevelWarn,
			expected: `level=WARN msg=msg pos=first
level=WARN msg=msg pos=second
level=WARN msg=msg pos=fourth
level=WARN msg=msg g.pos=after
`,
		},
		{
			description: "level info",
			level:       slog.LevelInfo,
			expected: `level=INFO msg=msg pos=first
level=INFO msg=msg pos=second
level=INFO msg=msg pos=fourth
level=INFO msg=msg g.pos=after
`,
		},
		{
			description: "level debug",
			level:       slog.LevelDebug,
			expected: `level=DEBUG msg=msg pos=first
level=DEBUG msg=msg pos=second
level=DEBUG msg=msg pos=fourth
level=DEBUG msg=msg g.pos=after
`,
		},
	}

	for _, testcase := range testcases {
		testcase := testcase

		t.Run(testcase.description, func(t *testing.T) {
			t.Parallel()

			buf := &bytes.Buffer{}
			handler := rate.New(
				slog.NewTextHandler(buf, &slog.HandlerOptions{
					Level: slog.LevelDebug,
					ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
						if len(groups) == 0 && attr.Key == slog.TimeKey {
							return slog.Attr{}
						}

						return attr
					},
				}),
				rate.WithFirst(2),
				rate.WithEvery(2),
				rate.WithInterval(time.Second),
			)
			logger := slog.New(handler)
			ctx := context.Background()

			logger.Log(ctx, testcase.level, "msg", "pos", "first")
			logger.Log(ctx, testcase.level, "msg", "pos", "second")
			logger.Log(ctx, testcase.level, "msg", "pos", "third")
			logger.Log(ctx, testcase.level, "msg", "pos", "fourth")
			time.Sleep(time.Second)
			logger.WithGroup("g").With("pos", "after").Log(ctx, testcase.level, "msg")

			assert.Equal(t, testcase.expected, buf.String())
		})
	}
}

func TestHandler_race(t *testing.T) {
	t.Parallel()

	procs := runtime.GOMAXPROCS(0)
	counter := atomic.Int64{}
	handler := rate.New(
		countHandler{count: &counter},
		rate.WithFirst(1),
		rate.WithEvery(1000),
	)
	logger := slog.New(handler)
	ctx := context.Background()

	start := make(chan struct{})
	var waitGroup sync.WaitGroup
	waitGroup.Add(procs)
	for i := 0; i < procs; i++ {
		go func() {
			defer waitGroup.Done()

			<-start
			logger.Log(ctx, slog.LevelInfo, "msg")
		}()
	}
	close(start)
	waitGroup.Wait()

	assert.Equal(t, 1, int(counter.Load()))
}

type countHandler struct {
	count *atomic.Int64
}

func (c countHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (c countHandler) Handle(context.Context, slog.Record) error {
	c.count.Add(1)

	return nil
}

func (c countHandler) WithAttrs([]slog.Attr) slog.Handler {
	return c
}

func (c countHandler) WithGroup(string) slog.Handler {
	return c
}
