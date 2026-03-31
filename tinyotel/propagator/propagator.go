package propagator

import (
	"fmt"
	"net/http"
	"strings"
)

// SpanContext holds the W3C trace context and baggage extracted from or
// injected into HTTP headers.
type SpanContext struct {
	TraceID string
	SpanID  string
	Sampled bool
	Baggage map[string]string
}

// Extract reads the traceparent and baggage headers from h and returns a
// SpanContext. Returns a zero-value SpanContext if the header is absent or
// malformed.
func Extract(h http.Header) SpanContext {
	var ctx SpanContext

	tp := h.Get("traceparent")
	if tp != "" {
		parts := strings.Split(tp, "-")
		if len(parts) == 4 && len(parts[1]) == 32 && len(parts[2]) == 16 {
			ctx.TraceID = parts[1]
			ctx.SpanID = parts[2]
			ctx.Sampled = parts[3] == "01"
		}
	}

	if b := h.Get("baggage"); b != "" {
		ctx.Baggage = make(map[string]string)
		for _, entry := range strings.Split(b, ",") {
			entry = strings.TrimSpace(entry)
			if k, v, ok := strings.Cut(entry, "="); ok {
				ctx.Baggage[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}

	return ctx
}

// Inject writes ctx into the traceparent (and baggage, if non-empty) headers of h.
func Inject(ctx SpanContext, h http.Header) {
	flags := "00"
	if ctx.Sampled {
		flags = "01"
	}
	h.Set("traceparent", fmt.Sprintf("00-%s-%s-%s", ctx.TraceID, ctx.SpanID, flags))

	if len(ctx.Baggage) > 0 {
		parts := make([]string, 0, len(ctx.Baggage))
		for k, v := range ctx.Baggage {
			parts = append(parts, k+"="+v)
		}
		h.Set("baggage", strings.Join(parts, ","))
	}
}
