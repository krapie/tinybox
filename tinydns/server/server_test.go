package server_test

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/krapi0314/tinybox/tinydns/registry"
	"github.com/krapi0314/tinybox/tinydns/server"
)

// queryUDP sends a DNS A query to addr and returns the response message.
func queryUDP(t *testing.T, addr, name string) *dns.Msg {
	t.Helper()
	c := &dns.Client{Net: "udp", Timeout: 3 * time.Second}
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), dns.TypeA)
	resp, _, err := c.Exchange(m, addr)
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}
	return resp
}

func queryTCP(t *testing.T, addr, name string) *dns.Msg {
	t.Helper()
	c := &dns.Client{Net: "tcp", Timeout: 3 * time.Second}
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), dns.TypeA)
	resp, _, err := c.Exchange(m, addr)
	if err != nil {
		t.Fatalf("DNS query (TCP) failed: %v", err)
	}
	return resp
}

// freePort picks an available UDP port on localhost.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := l.LocalAddr().(*net.UDPAddr).Port
	_ = l.Close()
	return port
}

func startServer(t *testing.T, reg *registry.Registry) string {
	t.Helper()
	port := freePort(t)
	addr := "127.0.0.1:" + strconv.Itoa(port)
	srv := server.New(addr, reg)
	if err := srv.Start(); err != nil {
		t.Fatalf("server.Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	// Give the server a moment to bind.
	time.Sleep(50 * time.Millisecond)
	return addr
}

func TestServerUDPResolveKnownName(t *testing.T) {
	reg := registry.New()
	reg.Register(registry.ServiceRecord{
		Name: "whoami.default.svc.cluster.local.",
		IP:   "172.19.0.2",
		TTL:  30,
	})

	addr := startServer(t, reg)
	resp := queryUDP(t, addr, "whoami.default.svc.cluster.local.")

	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("Rcode = %d, want NOERROR", resp.Rcode)
	}
	if len(resp.Answer) == 0 {
		t.Fatal("no answer records")
	}
	a, ok := resp.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("answer[0] is not an A record")
	}
	if a.A.String() != "172.19.0.2" {
		t.Errorf("A = %q, want 172.19.0.2", a.A.String())
	}
}

func TestServerUDPNXDOMAIN(t *testing.T) {
	reg := registry.New()
	addr := startServer(t, reg)
	resp := queryUDP(t, addr, "unknown.default.svc.cluster.local.")

	if resp.Rcode != dns.RcodeNameError {
		t.Errorf("Rcode = %d, want NXDOMAIN (%d)", resp.Rcode, dns.RcodeNameError)
	}
}

func TestServerTCPResolveKnownName(t *testing.T) {
	reg := registry.New()
	reg.Register(registry.ServiceRecord{
		Name: "api.default.svc.cluster.local.",
		IP:   "10.0.0.5",
		TTL:  30,
	})

	addr := startServer(t, reg)
	resp := queryTCP(t, addr, "api.default.svc.cluster.local.")

	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("Rcode = %d, want NOERROR", resp.Rcode)
	}
	if len(resp.Answer) == 0 {
		t.Fatal("no answer records")
	}
	a := resp.Answer[0].(*dns.A)
	if a.A.String() != "10.0.0.5" {
		t.Errorf("A = %q, want 10.0.0.5", a.A.String())
	}
}

func TestServerMultipleARecords(t *testing.T) {
	reg := registry.New()
	reg.Register(registry.ServiceRecord{Name: "svc.default.svc.cluster.local.", IP: "10.0.0.1", TTL: 30})
	reg.Register(registry.ServiceRecord{Name: "svc.default.svc.cluster.local.", IP: "10.0.0.2", TTL: 30})
	reg.Register(registry.ServiceRecord{Name: "svc.default.svc.cluster.local.", IP: "10.0.0.3", TTL: 30})

	addr := startServer(t, reg)
	resp := queryUDP(t, addr, "svc.default.svc.cluster.local.")

	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("Rcode = %d, want NOERROR", resp.Rcode)
	}
	if len(resp.Answer) != 3 {
		t.Errorf("answer count = %d, want 3", len(resp.Answer))
	}
}
