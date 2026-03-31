// Package config parses the tinyotel YAML configuration file.
// It implements a minimal line-by-line parser covering the exact subset of
// YAML used by the tinyotel config format — no external dependencies required.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/krapi0314/tinybox/tinyotel/processor"
)

// Config holds all tinyotel configuration.
type Config struct {
	Receiver ReceiverConfig
	Pipeline PipelineConfig
	Storage  StorageConfig
	API      APIConfig
}

// ReceiverConfig configures the OTLP/HTTP receiver.
type ReceiverConfig struct {
	HTTPPort int
}

// PipelineConfig lists the ordered processors.
type PipelineConfig struct {
	Processors []ProcessorConfig
}

// ProcessorConfig describes one processor in the pipeline.
type ProcessorConfig struct {
	Type          string
	MaxSize       int
	FlushInterval time.Duration
	Rules         []processor.AttributeRule
	SamplingRate  float64
}

// StorageConfig configures in-memory retention windows.
type StorageConfig struct {
	TraceRetention   time.Duration
	MetricsRetention time.Duration
	LogMaxRecords    int
}

// APIConfig configures the query API.
type APIConfig struct {
	HTTPPort int
}

// Default returns a Config with sensible defaults.
func Default() (*Config, error) {
	return &Config{
		Receiver: ReceiverConfig{HTTPPort: 4318},
		Pipeline: PipelineConfig{
			Processors: []ProcessorConfig{
				{Type: "batch", MaxSize: 512, FlushInterval: 5 * time.Second},
				{Type: "sampling", SamplingRate: 1.0},
			},
		},
		Storage: StorageConfig{
			TraceRetention:   time.Hour,
			MetricsRetention: 2 * time.Hour,
			LogMaxRecords:    100000,
		},
		API: APIConfig{HTTPPort: 4319},
	}, nil
}

// Parse reads a YAML config from r.
func Parse(r io.Reader) (*Config, error) {
	cfg, _ := Default()

	lines, err := readLines(r)
	if err != nil {
		return nil, err
	}

	// Tokenise into (indent, key, value) triples.
	type token struct {
		indent int
		key    string
		value  string
	}
	var tokens []token
	for _, line := range lines {
		stripped := strings.TrimLeft(line, " \t")
		if stripped == "" || strings.HasPrefix(stripped, "#") {
			continue
		}
		if strings.HasPrefix(stripped, "- ") {
			// list item — handled specially
			indent := len(line) - len(stripped)
			rest := strings.TrimPrefix(stripped, "- ")
			tokens = append(tokens, token{indent: indent, key: "-", value: rest})
			continue
		}
		colon := strings.Index(stripped, ":")
		if colon < 0 {
			return nil, fmt.Errorf("invalid config line: %q", line)
		}
		k := strings.TrimSpace(stripped[:colon])
		v := strings.TrimSpace(stripped[colon+1:])
		// strip inline comments
		if idx := strings.Index(v, " #"); idx >= 0 {
			v = strings.TrimSpace(v[:idx])
		}
		v = strings.Trim(v, `"'`)
		indent := len(line) - len(stripped)
		tokens = append(tokens, token{indent: indent, key: k, value: v})
	}

	// Walk tokens and populate cfg.
	i := 0
	for i < len(tokens) {
		t := tokens[i]
		switch t.key {
		case "receiver":
			i++
			for i < len(tokens) && tokens[i].indent > t.indent {
				switch tokens[i].key {
				case "http_port":
					cfg.Receiver.HTTPPort = mustInt(tokens[i].value)
				}
				i++
			}
		case "api":
			i++
			for i < len(tokens) && tokens[i].indent > t.indent {
				switch tokens[i].key {
				case "http_port":
					cfg.API.HTTPPort = mustInt(tokens[i].value)
				}
				i++
			}
		case "storage":
			i++
			for i < len(tokens) && tokens[i].indent > t.indent {
				switch tokens[i].key {
				case "trace_retention":
					cfg.Storage.TraceRetention = mustDuration(tokens[i].value)
				case "metrics_retention":
					cfg.Storage.MetricsRetention = mustDuration(tokens[i].value)
				case "log_max_records":
					cfg.Storage.LogMaxRecords = mustInt(tokens[i].value)
				}
				i++
			}
		case "pipeline":
			i++
			for i < len(tokens) && tokens[i].indent > t.indent {
				if tokens[i].key == "processors" {
					i++
					cfg.Pipeline.Processors = nil
					var cur *ProcessorConfig
					for i < len(tokens) && tokens[i].indent > t.indent {
						tt := tokens[i]
						if tt.key == "-" {
							// new processor block: "- type: batch"
							if cur != nil {
								cfg.Pipeline.Processors = append(cfg.Pipeline.Processors, *cur)
							}
							cur = &ProcessorConfig{}
							kv := strings.SplitN(tt.value, ":", 2)
							if len(kv) == 2 && strings.TrimSpace(kv[0]) == "type" {
								cur.Type = strings.TrimSpace(strings.Trim(kv[1], `"'`))
							}
							i++
						} else {
							if cur == nil {
								cur = &ProcessorConfig{}
							}
							switch tt.key {
							case "type":
								cur.Type = tt.value
							case "max_size":
								cur.MaxSize = mustInt(tt.value)
							case "flush_interval":
								cur.FlushInterval = mustDuration(tt.value)
							case "sampling_rate":
								cur.SamplingRate = mustFloat(tt.value)
							case "rules":
								i++
								for i < len(tokens) && tokens[i].indent > tt.indent {
									r := tokens[i]
									if r.key == "-" {
										rule := processor.AttributeRule{}
										kv := strings.SplitN(r.value, ":", 2)
										if len(kv) == 2 && strings.TrimSpace(kv[0]) == "action" {
											rule.Action = strings.TrimSpace(kv[1])
										}
										i++
										for i < len(tokens) && tokens[i].indent > r.indent {
											switch tokens[i].key {
											case "action":
												rule.Action = tokens[i].value
											case "key":
												rule.Key = tokens[i].value
											case "value":
												rule.Value = tokens[i].value
											case "new_key":
												rule.NewKey = tokens[i].value
											}
											i++
										}
										cur.Rules = append(cur.Rules, rule)
									} else {
										i++
									}
								}
								continue
							}
							i++
						}
					}
					if cur != nil {
						cfg.Pipeline.Processors = append(cfg.Pipeline.Processors, *cur)
					}
				} else {
					i++
				}
			}
		default:
			i++
		}
	}

	return cfg, nil
}

func readLines(r io.Reader) ([]string, error) {
	var lines []string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, errors.New("empty config")
	}
	// Quick YAML validity check: reject lines with bare tab-colon which is invalid.
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), ":\t") {
			return nil, fmt.Errorf("invalid YAML: %q", l)
		}
	}
	return lines, nil
}

func mustInt(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

func mustFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func mustDuration(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	return d
}
