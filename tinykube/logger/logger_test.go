package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestInfoAlwaysLogs(t *testing.T) {
	var buf bytes.Buffer
	l := newWithWriter(false, &buf)
	l.Info("hello %s", "world")
	if !strings.Contains(buf.String(), "[INFO]") {
		t.Errorf("expected [INFO] in output, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "hello world") {
		t.Errorf("expected message in output, got: %s", buf.String())
	}
}

func TestDebugSuppressedWhenDisabled(t *testing.T) {
	var buf bytes.Buffer
	l := newWithWriter(false, &buf)
	l.Debug("secret %s", "stuff")
	if buf.Len() != 0 {
		t.Errorf("expected no output when debug=false, got: %s", buf.String())
	}
}

func TestDebugEmittedWhenEnabled(t *testing.T) {
	var buf bytes.Buffer
	l := newWithWriter(true, &buf)
	l.Debug("reconcile %d deployments", 3)
	if !strings.Contains(buf.String(), "[DEBUG]") {
		t.Errorf("expected [DEBUG] in output, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "reconcile 3 deployments") {
		t.Errorf("expected message in output, got: %s", buf.String())
	}
}

func TestNopDiscardsEverything(t *testing.T) {
	l := NewNop()
	// Must not panic and must produce no observable output.
	l.Info("should be discarded")
	l.Debug("also discarded")
}
