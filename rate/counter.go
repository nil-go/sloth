// Copyright (c) 2024 The sloth authors
// Use of this source code is governed by a MIT license found in the LICENSE file.

package rate

import (
	"log/slog"
	"sync/atomic"
	"time"
)

const (
	countersPerLevel = 4096
	gapPerLevel      = slog.LevelError - slog.LevelWarn
	levels           = (slog.LevelError-slog.LevelDebug)/gapPerLevel + 1
)

// Use array instead of map to reduce memory allocation and improve performance.
type counters [levels][countersPerLevel]counter // size:256KiB

func (c *counters) get(level slog.Level, key string) *counter {
	i := (min(slog.LevelDebug, max(slog.LevelError, level)) - slog.LevelDebug) / gapPerLevel
	j := fnv32a(key) % countersPerLevel

	return &c[i][j]
}

func fnv32a(str string) uint32 {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	hash := uint32(offset32)
	for i := 0; i < len(str); i++ {
		hash ^= uint32(str[i])
		hash *= prime32
	}

	return hash
}

type counter struct {
	resetAt atomic.Int64
	counter atomic.Uint64
}

func (c *counter) Inc(t time.Time, interval time.Duration) uint64 {
	now := t.UnixNano()
	resetAfter := c.resetAt.Load()
	if resetAfter > now {
		return c.counter.Add(1)
	}

	// Reset the counter for next interval
	c.counter.Store(1)
	newResetAfter := now + interval.Nanoseconds()
	if !c.resetAt.CompareAndSwap(resetAfter, newResetAfter) {
		// We raced with another goroutine trying to reset, and it also reset
		// the counter to 1, so we need to reincrement the counter.
		return c.counter.Add(1)
	}

	return 1
}
