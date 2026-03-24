package plugins

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/miekg/dns"
)

// Log is a plugin that logs each DNS query with client IP, query name, type,
// response code, and latency, then delegates to Next.
type Log struct {
	next Plugin
	out  io.Writer
}

// NewLog creates a Log plugin that writes log lines to out and passes queries
// to next.
func NewLog(next Plugin, out io.Writer) *Log {
	return &Log{next: next, out: out}
}

func (l *Log) Name() string { return "log" }

func (l *Log) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	start := time.Now()
	rw := &capturingWriter{ResponseWriter: w}

	rcode, err := l.next.ServeDNS(ctx, rw, r)

	qname := "."
	qtype := "?"
	if len(r.Question) > 0 {
		qname = r.Question[0].Name
		qtype = dns.TypeToString[r.Question[0].Qtype]
	}

	rcodeStr := dns.RcodeToString[rcode]
	latency := time.Since(start)
	fmt.Fprintf(l.out, "%s %s %s %s %s\n",
		w.RemoteAddr(), qname, qtype, rcodeStr, latency)

	return rcode, err
}

// capturingWriter wraps a ResponseWriter so that the Log plugin can observe
// what rcode the downstream plugins set — it forwards all writes unchanged.
type capturingWriter struct {
	dns.ResponseWriter
	rcode int
}

func (c *capturingWriter) WriteMsg(m *dns.Msg) error {
	c.rcode = m.Rcode
	return c.ResponseWriter.WriteMsg(m)
}
