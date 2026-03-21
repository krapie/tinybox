package logger

import (
	"io"
	"log"
	"os"
)

// Logger is a two-level logger (INFO / DEBUG) backed by stdlib log.
// Debug output is gated by the debug flag; Info output is always emitted.
type Logger struct {
	inner *log.Logger
	debug bool
}

// New creates a Logger that writes to stdout.
// Pass debug=true to enable DEBUG lines.
func New(debug bool) *Logger {
	return newWithWriter(debug, os.Stdout)
}

// NewNop returns a Logger that discards all output — for use in unit tests.
func NewNop() *Logger {
	return newWithWriter(false, io.Discard)
}

// newWithWriter creates a Logger writing to w — used internally and in tests.
func newWithWriter(debug bool, w io.Writer) *Logger {
	return &Logger{
		inner: log.New(w, "", log.LstdFlags),
		debug: debug,
	}
}

// Info logs a message at INFO level (always emitted).
func (l *Logger) Info(format string, args ...interface{}) {
	l.inner.Printf("[INFO]  "+format, args...)
}

// Debug logs a message at DEBUG level (only emitted when debug=true).
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.debug {
		l.inner.Printf("[DEBUG] "+format, args...)
	}
}
