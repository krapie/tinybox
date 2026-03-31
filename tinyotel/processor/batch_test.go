package processor_test

import (
	"testing"
	"time"

	"github.com/krapi0314/tinybox/tinyotel/model"
	"github.com/krapi0314/tinybox/tinyotel/processor"
)

func makeSpan(traceID, spanID string) model.Span {
	return model.Span{TraceID: model.TraceID(traceID), SpanID: model.SpanID(spanID)}
}

func TestBatchFlushOnMaxSize(t *testing.T) {
	counter := &countingProcessor{}
	bp := processor.NewBatch(processor.NewChain(counter), 3, time.Minute)
	defer bp.Stop()

	bp.Process([]model.Span{makeSpan("t1", "s1"), makeSpan("t1", "s2")})
	if counter.count != 0 {
		t.Errorf("flushed early: count = %d, want 0", counter.count)
	}

	bp.Process([]model.Span{makeSpan("t2", "s3")})
	// Now at 3 — should have flushed.
	if counter.count != 3 {
		t.Errorf("after max size: count = %d, want 3", counter.count)
	}
}

func TestBatchFlushOnInterval(t *testing.T) {
	counter := &countingProcessor{}
	bp := processor.NewBatch(processor.NewChain(counter), 100, 50*time.Millisecond)
	defer bp.Stop()

	bp.Process([]model.Span{makeSpan("t1", "s1")})
	if counter.count != 0 {
		t.Errorf("flushed early: count = %d, want 0", counter.count)
	}

	time.Sleep(100 * time.Millisecond)
	if counter.count != 1 {
		t.Errorf("after interval: count = %d, want 1", counter.count)
	}
}

func TestBatchFlushOnStop(t *testing.T) {
	counter := &countingProcessor{}
	bp := processor.NewBatch(processor.NewChain(counter), 100, time.Minute)

	bp.Process([]model.Span{makeSpan("t1", "s1"), makeSpan("t2", "s2")})
	if counter.count != 0 {
		t.Errorf("flushed early: count = %d, want 0", counter.count)
	}
	bp.Stop()
	if counter.count != 2 {
		t.Errorf("after Stop: count = %d, want 2", counter.count)
	}
}

func TestBatchEmptyFlushIsNoop(t *testing.T) {
	counter := &countingProcessor{}
	bp := processor.NewBatch(processor.NewChain(counter), 3, 50*time.Millisecond)
	defer bp.Stop()

	time.Sleep(100 * time.Millisecond)
	if counter.count != 0 {
		t.Errorf("empty flush should not call next: count = %d", counter.count)
	}
}
