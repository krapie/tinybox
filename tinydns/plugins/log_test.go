package plugins_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/miekg/dns"

	"github.com/krapi0314/tinybox/tinydns/plugins"
)

func TestLogPluginCallsNext(t *testing.T) {
	next := &noopPlugin{rcode: dns.RcodeSuccess}
	lp := plugins.NewLog(next, &bytes.Buffer{})

	fw := &fakeWriter{}
	req := makeAQuery("whoami.default.svc.cluster.local.")
	_, err := lp.ServeDNS(context.Background(), fw, req)
	if err != nil {
		t.Fatalf("ServeDNS returned error: %v", err)
	}
	if next.called != 1 {
		t.Errorf("next.called = %d, want 1", next.called)
	}
}

func TestLogPluginWritesLogLine(t *testing.T) {
	next := &noopPlugin{rcode: dns.RcodeSuccess}
	var buf bytes.Buffer
	lp := plugins.NewLog(next, &buf)

	fw := &fakeWriter{}
	req := makeAQuery("api.default.svc.cluster.local.")
	lp.ServeDNS(context.Background(), fw, req)

	line := buf.String()
	if !strings.Contains(line, "api.default.svc.cluster.local.") {
		t.Errorf("log line missing query name: %q", line)
	}
	if !strings.Contains(line, "A") {
		t.Errorf("log line missing query type: %q", line)
	}
}

func TestLogPluginLogsNXDOMAIN(t *testing.T) {
	next := &noopPlugin{rcode: dns.RcodeNameError}
	var buf bytes.Buffer
	lp := plugins.NewLog(next, &buf)

	fw := &fakeWriter{}
	req := makeAQuery("unknown.svc.cluster.local.")
	lp.ServeDNS(context.Background(), fw, req)

	line := buf.String()
	if !strings.Contains(line, "NXDOMAIN") {
		t.Errorf("log line missing NXDOMAIN: %q", line)
	}
}
