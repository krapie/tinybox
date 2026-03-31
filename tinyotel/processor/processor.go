// Package processor defines the SpanProcessor interface and the Chain that
// runs processors in order, passing spans from one to the next.
package processor

import "github.com/krapi0314/tinybox/tinyotel/model"

// SpanProcessor transforms or filters a batch of spans.
type SpanProcessor interface {
	Process(spans []model.Span) []model.Span
}

// FuncProcessor adapts a plain function to the SpanProcessor interface.
type FuncProcessor func([]model.Span) []model.Span

func (f FuncProcessor) Process(spans []model.Span) []model.Span { return f(spans) }

// Chain runs a fixed sequence of SpanProcessors, piping the output of each
// into the input of the next.
type Chain struct {
	procs []SpanProcessor
}

// NewChain creates a Chain from the given processors (left to right).
func NewChain(procs ...SpanProcessor) *Chain {
	return &Chain{procs: procs}
}

// Process runs spans through every processor in order.
func (c *Chain) Process(spans []model.Span) []model.Span {
	for _, p := range c.procs {
		if len(spans) == 0 {
			break
		}
		spans = p.Process(spans)
	}
	return spans
}
