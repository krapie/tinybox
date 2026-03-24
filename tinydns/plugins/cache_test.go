package plugins_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/krapi0314/tinybox/tinydns/plugins"
)

func TestCacheMissCallsNext(t *testing.T) {
	next := &noopPlugin{rcode: dns.RcodeSuccess}
	cp := plugins.NewCache(next, 30*time.Second)

	fw := &fakeWriter{}
	req := makeAQuery("svc.default.svc.cluster.local.")
	cp.ServeDNS(context.Background(), fw, req)

	if next.called != 1 {
		t.Errorf("cache miss: next.called = %d, want 1", next.called)
	}
}

func TestCacheHitSkipsNext(t *testing.T) {
	// Build a response with an A record to cache.
	next := &noopPlugin{}
	next.rcode = dns.RcodeSuccess

	// Override next to write a real A record response.
	aNext := &aRecordPlugin{ip: "10.0.0.1", ttl: 30}
	cp := plugins.NewCache(aNext, 30*time.Second)

	fw := &fakeWriter{}
	req := makeAQuery("cached.default.svc.cluster.local.")

	// First call — cache miss, populates cache.
	cp.ServeDNS(context.Background(), fw, req)
	if aNext.called != 1 {
		t.Fatalf("first call: aNext.called = %d, want 1", aNext.called)
	}

	// Second call — should be a cache hit.
	fw2 := &fakeWriter{}
	cp.ServeDNS(context.Background(), fw2, req)
	if aNext.called != 1 {
		t.Errorf("cache hit: aNext.called = %d, want 1 (should not call next again)", aNext.called)
	}
	if fw2.msg == nil {
		t.Fatal("cache hit: no response written")
	}
	if fw2.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("cache hit: Rcode = %d, want NOERROR", fw2.msg.Rcode)
	}
}

func TestCacheTTLExpiry(t *testing.T) {
	aNext := &aRecordPlugin{ip: "10.0.0.2", ttl: 1}
	cp := plugins.NewCache(aNext, 50*time.Millisecond) // very short cache TTL

	fw := &fakeWriter{}
	req := makeAQuery("ttl.default.svc.cluster.local.")

	cp.ServeDNS(context.Background(), fw, req)
	if aNext.called != 1 {
		t.Fatalf("first call: aNext.called = %d, want 1", aNext.called)
	}

	// Wait for cache entry to expire.
	time.Sleep(100 * time.Millisecond)

	fw2 := &fakeWriter{}
	cp.ServeDNS(context.Background(), fw2, req)
	if aNext.called != 2 {
		t.Errorf("after TTL expiry: aNext.called = %d, want 2", aNext.called)
	}
}

// aRecordPlugin is a test plugin that always returns a single A record.
type aRecordPlugin struct {
	ip     string
	ttl    uint32
	called int
}

func (a *aRecordPlugin) Name() string { return "arecord" }
func (a *aRecordPlugin) ServeDNS(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	a.called++
	m := new(dns.Msg)
	m.SetReply(r)
	m.Answer = append(m.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   r.Question[0].Name,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    a.ttl,
		},
		A: net.ParseIP(a.ip).To4(),
	})
	_ = w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

var _ plugins.Plugin = (*aRecordPlugin)(nil)
