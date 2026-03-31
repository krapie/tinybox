package processor

import (
	"sync"
	"time"

	"github.com/krapi0314/tinybox/tinyotel/model"
)

// Batch accumulates spans and flushes them to the downstream processor either
// when the buffer reaches maxSize spans or the flushInterval elapses.
type Batch struct {
	next          SpanProcessor
	maxSize       int
	flushInterval time.Duration

	mu     sync.Mutex
	buf    []model.Span
	stopCh chan struct{}
	doneCh chan struct{}
}

// NewBatch creates a BatchProcessor. Call Stop() to flush remaining spans and
// shut down the background ticker.
func NewBatch(next SpanProcessor, maxSize int, flushInterval time.Duration) *Batch {
	b := &Batch{
		next:          next,
		maxSize:       maxSize,
		flushInterval: flushInterval,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
	go b.run()
	return b
}

// Process appends spans to the buffer, flushing immediately if maxSize is reached.
func (b *Batch) Process(spans []model.Span) []model.Span {
	b.mu.Lock()
	b.buf = append(b.buf, spans...)
	if len(b.buf) >= b.maxSize {
		b.flushLocked()
	}
	b.mu.Unlock()
	return nil // Batch never returns spans — they go to next on flush.
}

// Stop flushes any remaining spans and shuts down the background goroutine.
func (b *Batch) Stop() {
	close(b.stopCh)
	<-b.doneCh
}

func (b *Batch) run() {
	defer close(b.doneCh)
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.mu.Lock()
			b.flushLocked()
			b.mu.Unlock()
		case <-b.stopCh:
			b.mu.Lock()
			b.flushLocked()
			b.mu.Unlock()
			return
		}
	}
}

// flushLocked sends buffered spans downstream. Must be called with b.mu held.
func (b *Batch) flushLocked() {
	if len(b.buf) == 0 {
		return
	}
	batch := b.buf
	b.buf = nil
	b.next.Process(batch)
}
