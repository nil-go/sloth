module github.com/nil-go/sloth/otel

go 1.21

require (
	go.opentelemetry.io/otel v1.27.0
	go.opentelemetry.io/otel/trace v1.27.0
)

retract v0.2.0 // wrong trace context key
