# Opinionated Go slog handlers

![Go Version](https://img.shields.io/github/go-mod/go-version/nil-go/sloth)
[![Go Reference](https://pkg.go.dev/badge/github.com/nil-go/sloth.svg)](https://pkg.go.dev/github.com/nil-go/sloth)
[![Build](https://github.com/nil-go/sloth/actions/workflows/test.yml/badge.svg)](https://github.com/nil-go/sloth/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/nil-go/sloth)](https://goreportcard.com/report/github.com/nil-go/sloth)
[![Coverage](https://codecov.io/gh/nil-go/sloth/branch/main/graph/badge.svg)](https://codecov.io/gh/nil-go/sloth)

Sloth provides opinionated slog handlers for major Cloud providers. It providers following slog handlers:

- The [`gcp`](gcp)  slog handler is designed to emit JSON logs to GCP Cloud Logging by following its strict schema.
It also supports Cloud Trace correlation and Error Reporting integration.
It does not need special format for logs emit to AWS Cloud Watch and Azure Monitor
since they are designed to accept any format of logs.

- The [`rate`](rate) slog handler is designed to limit logs within the given rate to prevent flooding
during traffic spikes or incidents. It should before the final slog handler that write logs to the final destination.

- The [`otel`](otel) slog handler is designed to correlate logs with Open Telemetry spans.
It also supports recording logs as span events/error events if enabled.

- The [`sampling`](sampling) slog handler is designed to sample logs under the given minimal level at request scope.
It discards unsampled logs with lower level unless the buffer is activated by Handler.WithBuffer.
However, It also supports logs unsampled logs with lower level if there is a log with the minimum level and above.
It's suggested to correlate with tracing sampling, so that the logs and traces are consistent sampled.
