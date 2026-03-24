// Package server implements a DNS server that resolves names from a registry.
// It listens on both UDP and TCP and handles A record queries.
package server

import (
	"net"

	"github.com/miekg/dns"

	"github.com/krapi0314/tinybox/tinydns/registry"
)

// Server is a DNS server backed by a Registry.
type Server struct {
	addr string
	reg  *registry.Registry
	udp  *dns.Server
	tcp  *dns.Server
}

// New creates a Server that will listen on addr (e.g. "127.0.0.1:53") and
// resolve queries using reg.
func New(addr string, reg *registry.Registry) *Server {
	return &Server{addr: addr, reg: reg}
}

// Start binds the UDP and TCP listeners and begins serving DNS queries.
// It returns once both servers are ready to accept connections.
func (s *Server) Start() error {
	mux := dns.NewServeMux()
	mux.HandleFunc(".", s.handle)

	udpReady := make(chan struct{})
	tcpReady := make(chan struct{})
	errCh := make(chan error, 2)

	s.udp = &dns.Server{
		Addr:              s.addr,
		Net:               "udp",
		Handler:           mux,
		NotifyStartedFunc: func() { close(udpReady) },
	}
	s.tcp = &dns.Server{
		Addr:              s.addr,
		Net:               "tcp",
		Handler:           mux,
		NotifyStartedFunc: func() { close(tcpReady) },
	}

	go func() {
		if err := s.udp.ListenAndServe(); err != nil {
			errCh <- err
		}
	}()
	go func() {
		if err := s.tcp.ListenAndServe(); err != nil {
			errCh <- err
		}
	}()

	// Wait for both to signal ready or one to error.
	for i := 0; i < 2; i++ {
		select {
		case <-udpReady:
			udpReady = nil
		case <-tcpReady:
			tcpReady = nil
		case err := <-errCh:
			return err
		}
	}
	return nil
}

// Stop shuts down both the UDP and TCP listeners.
func (s *Server) Stop() error {
	if s.udp != nil {
		_ = s.udp.Shutdown()
	}
	if s.tcp != nil {
		_ = s.tcp.Shutdown()
	}
	return nil
}

// handle is the DNS request handler. It supports only A queries; anything else
// gets SERVFAIL. Known names return NOERROR with all A records; unknown names
// return NXDOMAIN.
func (s *Server) handle(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	if len(r.Question) == 0 {
		m.Rcode = dns.RcodeServerFailure
		_ = w.WriteMsg(m)
		return
	}

	q := r.Question[0]
	if q.Qtype != dns.TypeA && q.Qtype != dns.TypeANY {
		m.Rcode = dns.RcodeSuccess
		_ = w.WriteMsg(m)
		return
	}

	name := dns.Fqdn(q.Name)
	records := s.reg.Lookup(name)

	if len(records) == 0 {
		m.Rcode = dns.RcodeNameError
		_ = w.WriteMsg(m)
		return
	}

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
}
