package plugins

import (
	"context"
	"net"

	"github.com/miekg/dns"

	"github.com/krapi0314/tinybox/tinydns/registry"
)

// RegistryPlugin resolves DNS A queries from the in-memory service registry.
// On a registry hit it writes the response and returns. On a miss it delegates
// to Next.
type RegistryPlugin struct {
	reg  *registry.Registry
	next Plugin
}

// NewRegistryPlugin creates a RegistryPlugin backed by reg, falling through to
// next on a registry miss.
func NewRegistryPlugin(reg *registry.Registry, next Plugin) *RegistryPlugin {
	return &RegistryPlugin{reg: reg, next: next}
}

func (rp *RegistryPlugin) Name() string { return "registry" }

func (rp *RegistryPlugin) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if len(r.Question) == 0 {
		return rp.next.ServeDNS(ctx, w, r)
	}

	q := r.Question[0]
	if q.Qtype != dns.TypeA && q.Qtype != dns.TypeANY {
		return rp.next.ServeDNS(ctx, w, r)
	}

	name := dns.Fqdn(q.Name)
	records := rp.reg.Lookup(name)
	if len(records) == 0 {
		return rp.next.ServeDNS(ctx, w, r)
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	for _, rec := range records {
		m.Answer = append(m.Answer, &dns.A{
			Hdr: dns.RR_Header{
				Name:   name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    rec.TTL,
			},
			A: net.ParseIP(rec.IP).To4(),
		})
	}
	_ = w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}
