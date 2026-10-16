package telemetry

import "testing"

func TestOtlpHostPort(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"http://otel-collector:4317", "otel-collector:4317"},
		{"https://otel.example:4317", "otel.example:4317"},
		{"otel-collector:4317", "otel-collector:4317"},
		{"  http://localhost:4317/ ", "localhost:4317"},
	}
	for _, tc := range tests {
		if got := otlpHostPort(tc.in); got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.in, got, tc.want)
		}
	}
}
