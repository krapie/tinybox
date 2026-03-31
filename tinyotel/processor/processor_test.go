package processor_test

import (
	"testing"

	"github.com/krapi0314/tinybox/tinyotel/model"
	"github.com/krapi0314/tinybox/tinyotel/processor"
)

// countingProcessor records how many spans it received.
type countingProcessor struct{ count int }

func (c *countingProcessor) Process(spans []model.Span) []model.Span {
	c.count += len(spans)
	return spans
}

func TestChainCallsProcessorsInOrder(t *testing.T) {
	order := []string{}

	a := processor.FuncProcessor(func(spans []model.Span) []model.Span {
		order = append(order, "a")
		return spans
	})
	b := processor.FuncProcessor(func(spans []model.Span) []model.Span {
		order = append(order, "b")
		return spans
	})

	chain := processor.NewChain(a, b)
	spans := []model.Span{{TraceID: "t1", SpanID: "s1"}}
	chain.Process(spans)

	if len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Errorf("chain order = %v, want [a b]", order)
	}
}

func TestChainPassesSpansThrough(t *testing.T) {
	counter := &countingProcessor{}
	chain := processor.NewChain(counter)

	spans := []model.Span{
		{TraceID: "t1", SpanID: "s1"},
		{TraceID: "t1", SpanID: "s2"},
	}
	result := chain.Process(spans)

	if counter.count != 2 {
		t.Errorf("counter.count = %d, want 2", counter.count)
	}
	if len(result) != 2 {
		t.Errorf("result len = %d, want 2", len(result))
	}
}

func TestChainMidProcessorCanDropSpans(t *testing.T) {
	// A processor that drops all spans.
	dropper := processor.FuncProcessor(func(_ []model.Span) []model.Span {
		return nil
	})
	counter := &countingProcessor{}
	chain := processor.NewChain(dropper, counter)

	chain.Process([]model.Span{{TraceID: "t1", SpanID: "s1"}})

	if counter.count != 0 {
		t.Errorf("counter should receive 0 spans after dropper, got %d", counter.count)
	}
}
