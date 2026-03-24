// Package plugins defines the Plugin interface and the chain wiring used
// by tinydns to process DNS queries through an ordered middleware stack.
package plugins

import (
	"context"

	"github.com/miekg/dns"
)

// Plugin is implemented by every link in the DNS middleware chain.
// ServeDNS handles the query, optionally modifies the response, and either
// writes a reply itself or delegates to the next plugin via its own ServeDNS.
// The returned int is the DNS response code (dns.RcodeSuccess, etc.).
type Plugin interface {
	Name() string
	ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error)
}
