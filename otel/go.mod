module github.com/nil-go/sloth/otel

go 1.22
toolchain go1.22.5

require (
	go.opentelemetry.io/otel v1.34.0
	go.opentelemetry.io/otel/trace v1.34.0
)

retract v0.2.0 // wrong trace context key
