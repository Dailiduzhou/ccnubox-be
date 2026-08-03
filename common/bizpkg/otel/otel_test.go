package otel

import (
	"testing"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

func TestNewResourceMergesWithSDKDefault(t *testing.T) {
	const serviceName = "test-service"

	res, err := newResource(serviceName)
	if err != nil {
		t.Fatalf("merge resource: %v", err)
	}

	value, ok := res.Set().Value(semconv.ServiceNameKey)
	if !ok {
		t.Fatal("service.name attribute is missing")
	}
	if got := value.AsString(); got != serviceName {
		t.Fatalf("service.name = %q, want %q", got, serviceName)
	}
	if got, want := res.SchemaURL(), resource.Default().SchemaURL(); got != want {
		t.Fatalf("schema URL = %q, want SDK default %q", got, want)
	}
}
