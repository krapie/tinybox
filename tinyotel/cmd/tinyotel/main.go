// Command tinyotel is the tinyotel all-in-one binary.
// It starts the OTLP/HTTP receiver, processor pipeline, and query API,
// serving everything from a single process.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/krapi0314/tinybox/tinyotel/api"
	"github.com/krapi0314/tinybox/tinyotel/config"
	"github.com/krapi0314/tinybox/tinyotel/model"
	"github.com/krapi0314/tinybox/tinyotel/processor"
	"github.com/krapi0314/tinybox/tinyotel/receiver"
	logstore "github.com/krapi0314/tinybox/tinyotel/store/logs"
	metricsstore "github.com/krapi0314/tinybox/tinyotel/store/metrics"
	tracestore "github.com/krapi0314/tinybox/tinyotel/store/trace"
	"github.com/krapi0314/tinybox/tinyotel/ui"
)

func main() {
	cfgFile := flag.String("config", "", "path to config file (default: built-in defaults)")
	flag.Parse()

	var cfg *config.Config
	var err error
	if *cfgFile != "" {
		f, err := os.Open(*cfgFile)
		if err != nil {
			log.Fatalf("open config: %v", err)
		}
		defer f.Close()
		cfg, err = config.Parse(f)
		if err != nil {
			log.Fatalf("parse config: %v", err)
		}
	} else {
		cfg, err = config.Default()
		if err != nil {
			log.Fatalf("default config: %v", err)
		}
	}

	// ── Stores ──────────────────────────────────────────────────────────────
	ts := tracestore.NewStore(cfg.Storage.TraceRetention)
	ms := metricsstore.NewStore(cfg.Storage.MetricsRetention)
	ls := logstore.NewStore(cfg.Storage.LogMaxRecords)

	// Background retention eviction every minute.
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for range t.C {
			ts.Evict()
			ms.Evict()
		}
	}()

	// ── Processor pipeline ───────────────────────────────────────────────────
	// Build a final processor that writes to the trace store.
	var pipe processor.SpanProcessor = processor.FuncProcessor(func(spans []model.Span) []model.Span {
		for _, sp := range spans {
			ts.Append(sp)
		}
		return spans
	})

	// Wrap with configured processors in reverse order (innermost→outermost).
	for i := len(cfg.Pipeline.Processors) - 1; i >= 0; i-- {
		pc := cfg.Pipeline.Processors[i]
		switch pc.Type {
		case "sampling":
			pipe = processor.NewChain(processor.NewSampling(pc.SamplingRate), pipe)
		case "attributes":
			pipe = processor.NewChain(processor.NewAttributes(pc.Rules), pipe)
		case "batch":
			maxSize := pc.MaxSize
			if maxSize <= 0 {
				maxSize = 512
			}
			interval := pc.FlushInterval
			if interval <= 0 {
				interval = 5 * time.Second
			}
			pipe = processor.NewBatch(pipe, maxSize, interval)
		}
	}
	_ = pipe // pipeline available for future wiring into receiver

	// ── OTLP receiver (port 4318) ────────────────────────────────────────────
	receiverAddr := fmt.Sprintf(":%d", cfg.Receiver.HTTPPort)
	log.Printf("tinyotel OTLP receiver  listening on %s", receiverAddr)
	go func() {
		if err := http.ListenAndServe(receiverAddr, receiver.NewHandler(ts, ms, ls)); err != nil {
			log.Fatalf("receiver: %v", err)
		}
	}()

	// ── Query API + UI (port 4319) ───────────────────────────────────────────
	apiHandler := api.NewHandler(ts, ms, ls)
	uiHandler := ui.Handler()

	apiMux := http.NewServeMux()
	apiMux.Handle("/api/", apiHandler)
	apiMux.Handle("/health", apiHandler)
	apiMux.Handle("/ui/", http.StripPrefix("/ui", uiHandler))
	apiMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/ui/", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})

	apiAddr := fmt.Sprintf(":%d", cfg.API.HTTPPort)
	log.Printf("tinyotel query API + UI  listening on %s", apiAddr)
	if err := http.ListenAndServe(apiAddr, apiMux); err != nil {
		log.Fatalf("api: %v", err)
	}
}
