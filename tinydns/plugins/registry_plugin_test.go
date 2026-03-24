package plugins_test

import (
	"context"
	"testing"

	"github.com/miekg/dns"

	"github.com/krapi0314/tinybox/tinydns/plugins"
	"github.com/krapi0314/tinybox/tinydns/registry"
)

func TestRegistryPluginHitReturnsA(t *testing.T) {
	reg := registry.New()
	reg.Register(registry.ServiceRecord{
		Name: "whoami.default.svc.cluster.local.",
		IP:   "172.19.0.5",
		TTL:  30,
	})

	next := &noopPlugin{rcode: dns.RcodeNameError}
	rp := plugins.NewRegistryPlugin(reg, next)

	fw := &fakeWriter{}
	req := makeAQuery("whoami.default.svc.cluster.local.")
	rcode, err := rp.ServeDNS(context.Background(), fw, req)
	if err != nil {
		t.Fatalf("ServeDNS: %v", err)
	}
	if rcode != dns.RcodeSuccess {
		t.Errorf("rcode = %d, want NOERROR", rcode)
	}
	if next.called != 0 {
		t.Errorf("next.called = %d, want 0 (registry hit should not call next)", next.called)
	}
	if fw.msg == nil || len(fw.msg.Answer) == 0 {
		t.Fatal("no answer records in response")
	}
	a, ok := fw.msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatal("answer[0] is not an A record")
	}
	if a.A.String() != "172.19.0.5" {
		t.Errorf("A = %q, want 172.19.0.5", a.A.String())
	}
}

func TestRegistryPluginMissCallsNext(t *testing.T) {
	reg := registry.New()
	next := &noopPlugin{rcode: dns.RcodeNameError}
	rp := plugins.NewRegistryPlugin(reg, next)

	fw := &fakeWriter{}
	req := makeAQuery("unknown.default.svc.cluster.local.")
	rp.ServeDNS(context.Background(), fw, req)

	if next.called != 1 {
		t.Errorf("next.called = %d, want 1 (registry miss should call next)", next.called)
	}
}

func TestRegistryPluginMultipleARecords(t *testing.T) {
	reg := registry.New()
	reg.Register(registry.ServiceRecord{Name: "svc.default.svc.cluster.local.", IP: "10.0.0.1", TTL: 30})
	reg.Register(registry.ServiceRecord{Name: "svc.default.svc.cluster.local.", IP: "10.0.0.2", TTL: 30})

	next := &noopPlugin{rcode: dns.RcodeNameError}
	rp := plugins.NewRegistryPlugin(reg, next)

	fw := &fakeWriter{}
	req := makeAQuery("svc.default.svc.cluster.local.")
	rp.ServeDNS(context.Background(), fw, req)

	if len(fw.msg.Answer) != 2 {
		t.Errorf("answer count = %d, want 2", len(fw.msg.Answer))
	}
}
