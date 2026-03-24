package config_test

import (
	"strings"
	"testing"

	"github.com/krapi0314/tinybox/tinydns/config"
)

const sampleConfig = `
# tinydns config
listen :5353
upstream 8.8.8.8:53

plugins {
  log
  cache ttl=30
  registry
  forward
  health :8080
}
`

func TestParseListenAndUpstream(t *testing.T) {
	cfg, err := config.Parse(strings.NewReader(sampleConfig))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Listen != ":5353" {
		t.Errorf("Listen = %q, want :5353", cfg.Listen)
	}
	if cfg.Upstream != "8.8.8.8:53" {
		t.Errorf("Upstream = %q, want 8.8.8.8:53", cfg.Upstream)
	}
}

func TestParsePluginOrder(t *testing.T) {
	cfg, err := config.Parse(strings.NewReader(sampleConfig))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []string{"log", "cache", "registry", "forward", "health"}
	if len(cfg.Plugins) != len(want) {
		t.Fatalf("Plugins count = %d, want %d; got %v", len(cfg.Plugins), len(want), cfg.Plugins)
	}
	for i, p := range cfg.Plugins {
		if p.Name != want[i] {
			t.Errorf("Plugins[%d].Name = %q, want %q", i, p.Name, want[i])
		}
	}
}

func TestParseCacheTTL(t *testing.T) {
	cfg, err := config.Parse(strings.NewReader(sampleConfig))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var cacheCfg *config.PluginConfig
	for i := range cfg.Plugins {
		if cfg.Plugins[i].Name == "cache" {
			cacheCfg = &cfg.Plugins[i]
		}
	}
	if cacheCfg == nil {
		t.Fatal("cache plugin not found in config")
	}
	if cacheCfg.Args["ttl"] != "30" {
		t.Errorf("cache ttl = %q, want 30", cacheCfg.Args["ttl"])
	}
}

func TestParseHealthAddr(t *testing.T) {
	cfg, err := config.Parse(strings.NewReader(sampleConfig))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var healthCfg *config.PluginConfig
	for i := range cfg.Plugins {
		if cfg.Plugins[i].Name == "health" {
			healthCfg = &cfg.Plugins[i]
		}
	}
	if healthCfg == nil {
		t.Fatal("health plugin not found in config")
	}
	if healthCfg.Args["addr"] != ":8080" {
		t.Errorf("health addr = %q, want :8080", healthCfg.Args["addr"])
	}
}

func TestParseIgnoresComments(t *testing.T) {
	cfg, err := config.Parse(strings.NewReader(sampleConfig))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Sanity: config parsed without error, no garbage from the comment line.
	_ = cfg
}

func TestParseInvalidDirectiveReturnsError(t *testing.T) {
	bad := `listen :5353
unknown_directive foo
`
	_, err := config.Parse(strings.NewReader(bad))
	if err == nil {
		t.Error("expected error for unknown directive, got nil")
	}
}
