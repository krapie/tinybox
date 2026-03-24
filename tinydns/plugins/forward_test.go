package plugins_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/krapi0314/tinybox/tinydns/plugins"
)

// startFakeUpstream starts a minimal UDP DNS server that always responds with
// the given A record for any query. Returns the server address.
func startFakeUpstream(t *testing.T, ip string) string {
	t.Helper()

	mux := dns.NewServeMux()
	mux.HandleFunc(".", func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		if len(r.Question) > 0 && r.Question[0].Qtype == dns.TypeA {
			m.Answer = append(m.Answer, &dns.A{
				Hdr: dns.RR_Header{
					Name:   r.Question[0].Name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    30,
				},
				A: net.ParseIP(ip).To4(),
			})
		}
		_ = w.WriteMsg(m)
	})

	l, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("startFakeUpstream listen: %v", err)
	}
	addr := l.LocalAddr().String()
	_ = l.Close()

	srv := &dns.Server{Addr: addr, Net: "udp", Handler: mux}
	ready := make(chan struct{})
	srv.NotifyStartedFunc = func() { close(ready) }
	go srv.ListenAndServe()
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("fake upstream did not start")
	}
	t.Cleanup(func() { _ = srv.Shutdown() })
	return addr
}

func TestForwardResolvesViaUpstream(t *testing.T) {
	upstreamAddr := startFakeUpstream(t, "1.2.3.4")

	fp := plugins.NewForward(upstreamAddr, 2*time.Second)

	fw := &fakeWriter{}
	req := makeAQuery("external.example.com.")
	rcode, err := fp.ServeDNS(context.Background(), fw, req)
	if err != nil {
		t.Fatalf("ServeDNS: %v", err)
	}
	if rcode != dns.RcodeSuccess {
		t.Errorf("rcode = %d, want NOERROR", rcode)
	}
	if fw.msg == nil || len(fw.msg.Answer) == 0 {
		t.Fatal("no answer from forward")
	}
	a, ok := fw.msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatal("answer[0] is not an A record")
	}
	if a.A.String() != "1.2.3.4" {
		t.Errorf("A = %q, want 1.2.3.4", a.A.String())
	}
}

func TestForwardTimeout(t *testing.T) {
	// Point at a non-listening port — should time out quickly.
	fp := plugins.NewForward("127.0.0.1:1", 100*time.Millisecond)

	fw := &fakeWriter{}
	req := makeAQuery("external.example.com.")
	rcode, _ := fp.ServeDNS(context.Background(), fw, req)
	if rcode != dns.RcodeServerFailure {
		t.Errorf("rcode = %d, want SERVFAIL on timeout", rcode)
	}
}
