package plugins

import (
	"context"
	"time"

	"github.com/miekg/dns"
)

// Forward is a terminal plugin that forwards DNS queries to an upstream resolver.
// It is intended to be the last link in the chain, handling queries that no
// earlier plugin could resolve.
type Forward struct {
	upstream string
	timeout  time.Duration
}

// NewForward creates a Forward plugin that sends queries to upstream (e.g.
// "8.8.8.8:53") with the given per-query timeout.
func NewForward(upstream string, timeout time.Duration) *Forward {
	return &Forward{upstream: upstream, timeout: timeout}
}

func (f *Forward) Name() string { return "forward" }

func (f *Forward) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	c := &dns.Client{Net: "udp", Timeout: f.timeout}
	resp, _, err := c.Exchange(r, f.upstream)
	if err != nil {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Rcode = dns.RcodeServerFailure
		_ = w.WriteMsg(m)
		return dns.RcodeServerFailure, nil
	}
	_ = w.WriteMsg(resp)
	return resp.Rcode, nil
}
