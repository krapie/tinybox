package apiserver

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

// responseRecorder wraps http.ResponseWriter to capture status code and bytes written.
type responseRecorder struct {
	http.ResponseWriter
	status int
	size   int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.size += n
	return n, err
}

// loggingMiddleware logs one line per request:
//
//	POST /apis/apps/v1/namespaces/default/deployments 201 312B 1.2ms
func loggingMiddleware(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Printf("%s %s %d %dB %s",
			r.Method,
			r.URL.Path,
			rec.status,
			rec.size,
			fmt.Sprintf("%.3fms", float64(time.Since(start).Microseconds())/1000),
		)
	})
}
