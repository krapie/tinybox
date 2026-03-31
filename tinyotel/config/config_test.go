package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/krapi0314/tinybox/tinyotel/config"
)

const sampleConfig = `
receiver:
  http_port: 4318

pipeline:
  processors:
    - type: batch
      max_size: 512
      flush_interval: 5s
    - type: attributes
      rules:
        - action: insert
          key: tinyotel.version
          value: "0.1"
    - type: sampling
      sampling_rate: 0.5

storage:
  trace_retention: 1h
  metrics_retention: 2h
  log_max_records: 100000

api:
  http_port: 4319
`

func TestParseReceiverPort(t *testing.T) {
	cfg, err := config.Parse(strings.NewReader(sampleConfig))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Receiver.HTTPPort != 4318 {
		t.Errorf("HTTPPort = %d, want 4318", cfg.Receiver.HTTPPort)
	}
}

func TestParseAPIPort(t *testing.T) {
	cfg, err := config.Parse(strings.NewReader(sampleConfig))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.API.HTTPPort != 4319 {
		t.Errorf("APIPort = %d, want 4319", cfg.API.HTTPPort)
	}
}

func TestParseStorageRetention(t *testing.T) {
	cfg, err := config.Parse(strings.NewReader(sampleConfig))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Storage.TraceRetention != time.Hour {
		t.Errorf("TraceRetention = %v, want 1h", cfg.Storage.TraceRetention)
	}
	if cfg.Storage.MetricsRetention != 2*time.Hour {
		t.Errorf("MetricsRetention = %v, want 2h", cfg.Storage.MetricsRetention)
	}
	if cfg.Storage.LogMaxRecords != 100000 {
		t.Errorf("LogMaxRecords = %d, want 100000", cfg.Storage.LogMaxRecords)
	}
}

func TestParsePipelineProcessors(t *testing.T) {
	cfg, err := config.Parse(strings.NewReader(sampleConfig))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Pipeline.Processors) != 3 {
		t.Fatalf("expected 3 processors, got %d", len(cfg.Pipeline.Processors))
	}

	batch := cfg.Pipeline.Processors[0]
	if batch.Type != "batch" {
		t.Errorf("type = %q, want batch", batch.Type)
	}
	if batch.MaxSize != 512 {
		t.Errorf("max_size = %d, want 512", batch.MaxSize)
	}
	if batch.FlushInterval != 5*time.Second {
		t.Errorf("flush_interval = %v, want 5s", batch.FlushInterval)
	}

	attrs := cfg.Pipeline.Processors[1]
	if attrs.Type != "attributes" {
		t.Errorf("type = %q, want attributes", attrs.Type)
	}
	if len(attrs.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(attrs.Rules))
	}
	if attrs.Rules[0].Action != "insert" || attrs.Rules[0].Key != "tinyotel.version" || attrs.Rules[0].Value != "0.1" {
		t.Errorf("unexpected rule: %+v", attrs.Rules[0])
	}

	sampling := cfg.Pipeline.Processors[2]
	if sampling.Type != "sampling" {
		t.Errorf("type = %q, want sampling", sampling.Type)
	}
	if sampling.SamplingRate != 0.5 {
		t.Errorf("sampling_rate = %v, want 0.5", sampling.SamplingRate)
	}
}

func TestParseDefaults(t *testing.T) {
	cfg, err := config.Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	if cfg.Receiver.HTTPPort == 0 {
		t.Error("default receiver port should not be 0")
	}
	if cfg.API.HTTPPort == 0 {
		t.Error("default api port should not be 0")
	}
}

func TestParseBadYAML(t *testing.T) {
	_, err := config.Parse(strings.NewReader(":\t:bad yaml"))
	if err == nil {
		t.Error("expected error for bad YAML")
	}
}
