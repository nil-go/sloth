# Opinionated Go slog handlers

![Go Version](https://img.shields.io/github/go-mod/go-version/nil-go/sloth)
[![Go Reference](https://pkg.go.dev/badge/github.com/nil-go/sloth.svg)](https://pkg.go.dev/github.com/nil-go/sloth)
[![Build](https://github.com/nil-go/sloth/actions/workflows/test.yml/badge.svg)](https://github.com/nil-go/sloth/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/nil-go/sloth)](https://goreportcard.com/report/github.com/nil-go/sloth)
[![Coverage](https://codecov.io/gh/nil-go/sloth/branch/main/graph/badge.svg)](https://codecov.io/gh/nil-go/sloth)

Sloth provides opinionated slog handlers for major Cloud providers. It providers following slog handlers:

- [`sampling`](sampling) provides a slog handler for sampling records at request level.
- [`gcp`](gcp) provides a slog handler for emitting JSON logs to GCP Cloud Logging.
