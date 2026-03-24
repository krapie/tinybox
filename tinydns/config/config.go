// Package config parses the tinydns configuration file.
//
// Format:
//
//	# comment
//	listen :5353
//	upstream 8.8.8.8:53
//
//	plugins {
//	  log
//	  cache ttl=30
//	  registry
//	  forward
//	  health :8080
//	}
package config

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// PluginConfig holds a plugin name and its key=value arguments.
// Positional arguments (like the health address ":8080") are stored under
// the key "addr".
type PluginConfig struct {
	Name string
	Args map[string]string
}

// Config is the parsed representation of a tinydns configuration file.
type Config struct {
	Listen   string         // e.g. ":5353"
	Upstream string         // e.g. "8.8.8.8:53"
	Plugins  []PluginConfig // ordered list of plugins
}

// Parse reads a tinydns config from r and returns the parsed Config.
func Parse(r io.Reader) (*Config, error) {
	cfg := &Config{
		Listen:   ":5353",
		Upstream: "8.8.8.8:53",
	}

	scanner := bufio.NewScanner(r)
	inPlugins := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip blank lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if line == "plugins {" {
			inPlugins = true
			continue
		}
		if line == "}" {
			inPlugins = false
			continue
		}

		if inPlugins {
			pc, err := parsePluginLine(line)
			if err != nil {
				return nil, err
			}
			cfg.Plugins = append(cfg.Plugins, pc)
			continue
		}

		// Top-level directives.
		fields := strings.Fields(line)
		switch fields[0] {
		case "listen":
			if len(fields) < 2 {
				return nil, fmt.Errorf("listen: missing address")
			}
			cfg.Listen = fields[1]
		case "upstream":
			if len(fields) < 2 {
				return nil, fmt.Errorf("upstream: missing address")
			}
			cfg.Upstream = fields[1]
		default:
			return nil, fmt.Errorf("unknown directive: %q", fields[0])
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// parsePluginLine parses a single line inside the plugins { } block.
// Lines look like:
//
//	log
//	cache ttl=30
//	health :8080
func parsePluginLine(line string) (PluginConfig, error) {
	fields := strings.Fields(line)
	pc := PluginConfig{Name: fields[0], Args: make(map[string]string)}

	for _, f := range fields[1:] {
		if strings.Contains(f, "=") {
			// key=value argument.
			parts := strings.SplitN(f, "=", 2)
			pc.Args[parts[0]] = parts[1]
		} else {
			// Positional argument — treat as "addr".
			pc.Args["addr"] = f
		}
	}
	return pc, nil
}
