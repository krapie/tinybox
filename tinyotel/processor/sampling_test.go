package processor_test

import (
	"fmt"
	"testing"

	"github.com/krapi0314/tinybox/tinyotel/model"
	"github.com/krapi0314/tinybox/tinyotel/processor"
)

func TestSamplingKeepAll(t *testing.T) {
	sp := processor.NewSampling(1.0)
	spans := make([]model.Span, 20)
	for i := range spans {
		spans[i] = makeSpan(fmt.Sprintf("trace%d", i), fmt.Sprintf("span%d", i))
	}
	result := sp.Process(spans)
	if len(result) != 20 {
		t.Errorf("rate=1.0: kept %d, want 20", len(result))
	}
}

func TestSamplingDropAll(t *testing.T) {
	sp := processor.NewSampling(0.0)
	spans := make([]model.Span, 20)
	for i := range spans {
		spans[i] = makeSpan(fmt.Sprintf("trace%d", i), fmt.Sprintf("span%d", i))
	}
	result := sp.Process(spans)
	if len(result) != 0 {
		t.Errorf("rate=0.0: kept %d, want 0", len(result))
	}
}

func TestSamplingConsistentPerTrace(t *testing.T) {
	// All spans with the same traceID must be kept or dropped together.
	sp := processor.NewSampling(0.5)

	traceIDs := []string{"aaaa", "bbbb", "cccc", "dddd", "eeee",
		"ffff", "1111", "2222", "3333", "4444"}

	for _, tid := range traceIDs {
		// Submit two spans for the same trace.
		spans := []model.Span{
			makeSpan(tid, "s1"),
			makeSpan(tid, "s2"),
		}
		result := sp.Process(spans)
		// Either both kept or both dropped — never split.
		if len(result) != 0 && len(result) != 2 {
			t.Errorf("traceID %s: inconsistent sampling — got %d spans", tid, len(result))
		}
	}
}

func TestSamplingApproximateRate(t *testing.T) {
	// With 1000 distinct traces at rate=0.5, expect roughly 40–60% kept.
	sp := processor.NewSampling(0.5)
	var spans []model.Span
	for i := 0; i < 1000; i++ {
		spans = append(spans, makeSpan(fmt.Sprintf("%032d", i), "s1"))
	}
	result := sp.Process(spans)
	rate := float64(len(result)) / 1000.0
	if rate < 0.35 || rate > 0.65 {
		t.Errorf("sampling rate=0.5: kept %.2f of traces (want 0.35–0.65)", rate)
	}
}
