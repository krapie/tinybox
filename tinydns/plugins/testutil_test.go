package plugins_test

import (
	"context"
	"net"

	"github.com/miekg/dns"

	"github.com/krapi0314/tinybox/tinydns/plugins"
)

// fakeWriter is a minimal dns.ResponseWriter that captures the written message.
type fakeWriter struct {
	msg *dns.Msg
}

func (f *fakeWriter) LocalAddr() net.Addr         { return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)} }
func (f *fakeWriter) RemoteAddr() net.Addr        { return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)} }
func (f *fakeWriter) WriteMsg(m *dns.Msg) error   { f.msg = m; return nil }
func (f *fakeWriter) Write(b []byte) (int, error) { return len(b), nil }
func (f *fakeWriter) Close() error                { return nil }
func (f *fakeWriter) TsigStatus() error           { return nil }
func (f *fakeWriter) TsigTimersOnly(bool)         {}
func (f *fakeWriter) Hijack()                     {}

// noopPlugin is a terminal plugin that returns the given rcode.
type noopPlugin struct {
	rcode   int
	called  int
}

func (n *noopPlugin) Name() string { return "noop" }
func (n *noopPlugin) ServeDNS(_ context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	n.called++
	m := new(dns.Msg)
	m.SetReply(r)
	m.Rcode = n.rcode
	_ = w.WriteMsg(m)
	return n.rcode, nil
}

// ensure noopPlugin satisfies the interface at compile time.
var _ plugins.Plugin = (*noopPlugin)(nil)

// makeAQuery builds a simple A query for name.
func makeAQuery(name string) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), dns.TypeA)
	return m
}
