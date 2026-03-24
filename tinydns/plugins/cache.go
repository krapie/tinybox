package plugins

import (
	"context"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type cacheKey struct {
	name  string
	qtype uint16
}

type cacheEntry struct {
	msg       *dns.Msg
	expiresAt time.Time
}

// Cache is a plugin that caches DNS responses for the duration of maxTTL.
// On a cache hit it serves the cached reply immediately without calling Next.
// On a cache miss it calls Next, caches a successful response, and returns.
type Cache struct {
	next   Plugin
	maxTTL time.Duration
	mu     sync.RWMutex
	store  map[cacheKey]cacheEntry
}

// NewCache creates a Cache plugin. maxTTL caps how long any entry is kept;
// entries are also evicted lazily on the next lookup after they expire.
func NewCache(next Plugin, maxTTL time.Duration) *Cache {
	return &Cache{
		next:   next,
		maxTTL: maxTTL,
		store:  make(map[cacheKey]cacheEntry),
	}
}

func (c *Cache) Name() string { return "cache" }

func (c *Cache) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if len(r.Question) == 0 {
		return c.next.ServeDNS(ctx, w, r)
	}

	key := cacheKey{name: r.Question[0].Name, qtype: r.Question[0].Qtype}

	// Check cache.
	c.mu.RLock()
	entry, ok := c.store[key]
	c.mu.RUnlock()

	if ok && time.Now().Before(entry.expiresAt) {
		m := entry.msg.Copy()
		m.SetReply(r)
		_ = w.WriteMsg(m)
		return m.Rcode, nil
	}

	// Cache miss — call downstream.
	cw := &cachingWriter{ResponseWriter: w}
	rcode, err := c.next.ServeDNS(ctx, cw, r)

	if cw.msg != nil && rcode == dns.RcodeSuccess {
		c.mu.Lock()
		c.store[key] = cacheEntry{
			msg:       cw.msg.Copy(),
			expiresAt: time.Now().Add(c.maxTTL),
		}
		c.mu.Unlock()
	}
	return rcode, err
}

// cachingWriter intercepts WriteMsg so Cache can inspect and store the response.
type cachingWriter struct {
	dns.ResponseWriter
	msg *dns.Msg
}

func (cw *cachingWriter) WriteMsg(m *dns.Msg) error {
	cw.msg = m
	return cw.ResponseWriter.WriteMsg(m)
}
