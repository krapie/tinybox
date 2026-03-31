package propagator_test

import (
	"net/http"
	"testing"

	"github.com/krapi0314/tinybox/tinyotel/propagator"
)

func TestExtractTraceparent(t *testing.T) {
	h := http.Header{}
	h.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")

	ctx := propagator.Extract(h)
	if ctx.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("TraceID = %q", ctx.TraceID)
	}
	if ctx.SpanID != "00f067aa0ba902b7" {
		t.Errorf("SpanID = %q", ctx.SpanID)
	}
	if !ctx.Sampled {
		t.Error("Sampled = false, want true")
	}
}

func TestExtractNotSampled(t *testing.T) {
	h := http.Header{}
	h.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00")

	ctx := propagator.Extract(h)
	if ctx.Sampled {
		t.Error("Sampled = true, want false (flags=00)")
	}
}

func TestExtractMissingHeader(t *testing.T) {
	ctx := propagator.Extract(http.Header{})
	if ctx.TraceID != "" {
		t.Errorf("expected empty TraceID for missing header, got %q", ctx.TraceID)
	}
}

func TestExtractMalformedHeader(t *testing.T) {
	h := http.Header{}
	h.Set("traceparent", "not-valid")
	ctx := propagator.Extract(h)
	if ctx.TraceID != "" {
		t.Errorf("expected empty TraceID for malformed header, got %q", ctx.TraceID)
	}
}

func TestInjectTraceparent(t *testing.T) {
	ctx := propagator.SpanContext{
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:  "00f067aa0ba902b7",
		Sampled: true,
	}
	h := http.Header{}
	propagator.Inject(ctx, h)

	want := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	if got := h.Get("traceparent"); got != want {
		t.Errorf("traceparent = %q, want %q", got, want)
	}
}

func TestInjectNotSampled(t *testing.T) {
	ctx := propagator.SpanContext{
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:  "00f067aa0ba902b7",
		Sampled: false,
	}
	h := http.Header{}
	propagator.Inject(ctx, h)

	tp := h.Get("traceparent")
	if tp[len(tp)-2:] != "00" {
		t.Errorf("flags should be 00 for not-sampled, traceparent = %q", tp)
	}
}

func TestExtractBaggage(t *testing.T) {
	h := http.Header{}
	h.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	h.Set("baggage", "userId=alice,serverNode=node1")

	ctx := propagator.Extract(h)
	if ctx.Baggage["userId"] != "alice" {
		t.Errorf("Baggage[userId] = %q, want alice", ctx.Baggage["userId"])
	}
	if ctx.Baggage["serverNode"] != "node1" {
		t.Errorf("Baggage[serverNode] = %q, want node1", ctx.Baggage["serverNode"])
	}
}

func TestInjectBaggage(t *testing.T) {
	ctx := propagator.SpanContext{
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:  "00f067aa0ba902b7",
		Sampled: true,
		Baggage: map[string]string{"env": "prod", "region": "us-east"},
	}
	h := http.Header{}
	propagator.Inject(ctx, h)

	baggage := h.Get("baggage")
	if baggage == "" {
		t.Fatal("baggage header not set")
	}
	// Both entries must be present (order may vary).
	for _, entry := range []string{"env=prod", "region=us-east"} {
		found := false
		for _, part := range splitBaggage(baggage) {
			if part == entry {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("baggage missing %q in %q", entry, baggage)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	original := propagator.SpanContext{
		TraceID: "aabbccddeeff00112233445566778899",
		SpanID:  "aabbccdd11223344",
		Sampled: true,
		Baggage: map[string]string{"key": "value"},
	}
	h := http.Header{}
	propagator.Inject(original, h)
	recovered := propagator.Extract(h)

	if recovered.TraceID != original.TraceID {
		t.Errorf("TraceID round-trip: got %q, want %q", recovered.TraceID, original.TraceID)
	}
	if recovered.SpanID != original.SpanID {
		t.Errorf("SpanID round-trip: got %q, want %q", recovered.SpanID, original.SpanID)
	}
	if recovered.Sampled != original.Sampled {
		t.Errorf("Sampled round-trip: got %v, want %v", recovered.Sampled, original.Sampled)
	}
	if recovered.Baggage["key"] != "value" {
		t.Errorf("Baggage round-trip: got %q", recovered.Baggage["key"])
	}
}

func splitBaggage(s string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			part := s[start:i]
			// trim whitespace
			for len(part) > 0 && (part[0] == ' ' || part[0] == '\t') {
				part = part[1:]
			}
			for len(part) > 0 && (part[len(part)-1] == ' ' || part[len(part)-1] == '\t') {
				part = part[:len(part)-1]
			}
			if part != "" {
				parts = append(parts, part)
			}
			start = i + 1
		}
	}
	return parts
}
