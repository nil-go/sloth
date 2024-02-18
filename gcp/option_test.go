// Copyright (c) 2024 The sloth authors
// Use of this source code is governed by a MIT license found in the LICENSE file.

package gcp_test

import (
	"context"
	"testing"

	"github.com/nil-go/sloth/gcp"
	"github.com/nil-go/sloth/internal/assert"
)

func TestOption_panic(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		description string
		option      func() gcp.Option
		err         string
	}{
		{
			description: "project is empty",
			option: func() gcp.Option {
				return gcp.WithTrace("", func(context.Context) gcp.TraceContext {
					return traceContext{}
				})
			},
			err: "cannot add trace information with empty project",
		},
		{
			description: "context provider is nil",
			option: func() gcp.Option {
				return gcp.WithTrace("test", nil)
			},
			err: "cannot add trace information with nil context provider",
		},
		{
			description: "service is empty",
			option: func() gcp.Option {
				return gcp.WithErrorReporting("", "dev")
			},
			err: "cannot add error information with empty service",
		},
	}

	for _, testcase := range testcases {
		testcase := testcase

		t.Run(testcase.description, func(t *testing.T) {
			t.Parallel()

			defer func() {
				assert.Equal(t, testcase.err, recover().(string))
			}()

			testcase.option()
			t.Fail()
		})
	}
}
