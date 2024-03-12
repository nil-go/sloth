# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres
to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.0] - 2024-03-11

## Changed

- Decouple WithBuffer from SamplingHandler (#30).

## [0.2.3] - 2024-03-11

### Fixed

- Pass through log with invalid trace context (#28).

## [0.2.2] - 2024-02-26

### Removed

- Remove panic from gcp options (#23).

## [0.2.1] - 2024-02-25

### Changed

- Change trace context fields name according to
[Trace Context in non-OTLP Log Formats](https://opentelemetry.io/docs/specs/otel/compatibility/logging_trace_context/) (#22).

## [0.2.0] - 2024-02-25

### Add

- Add handler for correlation between log records and Open Telemetry spans (#13).
- Add gcp.WithCallers to retrieve stack trace from error (#14).

## [0.1.1] - 2024-02-23

### Changed

- Reduce heap escapes (#11).

## [0.1.0] - 2024-02-22

### Added

- Add sampling handler for sampling records at request level (#3).
- Add handler to emit JSON logs to GCP Cloud Logging (#6).
- Add handler to limit records with give rate (#8).
