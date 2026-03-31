package processor

import (
	"hash/fnv"

	"github.com/krapi0314/tinybox/tinyotel/model"
)

// Sampling implements head-based probability sampling with a consistent
// per-trace decision: all spans sharing a traceID are either kept or dropped.
type Sampling struct {
	rate float64 // 0.0 = drop all, 1.0 = keep all
}

// NewSampling creates a SamplingProcessor. rate must be in [0.0, 1.0].
func NewSampling(rate float64) *Sampling {
	return &Sampling{rate: rate}
}

func (s *Sampling) Process(spans []model.Span) []model.Span {
	if s.rate >= 1.0 {
		return spans
	}
	if s.rate <= 0.0 {
		return nil
	}

	// Compute per-trace keep decisions (consistent: same traceID → same result).
	keep := make(map[model.TraceID]bool, len(spans))
	for _, sp := range spans {
		if _, decided := keep[sp.TraceID]; !decided {
			keep[sp.TraceID] = s.shouldKeep(sp.TraceID)
		}
	}

	out := spans[:0]
	for _, sp := range spans {
		if keep[sp.TraceID] {
			out = append(out, sp)
		}
	}
	return out
}

// shouldKeep returns true if the traceID falls within the sampling rate bucket.
// Uses FNV-32a for a fast, deterministic, uniformly-distributed hash.
func (s *Sampling) shouldKeep(traceID model.TraceID) bool {
	h := fnv.New32a()
	h.Write([]byte(traceID))
	// Map hash to [0, 10000) and compare against rate*10000.
	return float64(h.Sum32()%10000) < s.rate*10000
}
