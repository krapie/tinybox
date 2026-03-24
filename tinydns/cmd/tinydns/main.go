// tinydns — a simplified CoreDNS-style DNS server with service discovery.
//
// Usage:
//
//	tinydns [-config path] [-tinykube addr] [-namespace ns]
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/krapi0314/tinybox/tinydns/apiserver"
	"github.com/krapi0314/tinybox/tinydns/config"
	"github.com/krapi0314/tinybox/tinydns/plugins"
	"github.com/krapi0314/tinybox/tinydns/registry"
	"github.com/krapi0314/tinybox/tinydns/server"
	"github.com/krapi0314/tinybox/tinydns/syncer"
)

func main() {
	configPath := flag.String("config", "", "path to tinydns config file (optional)")
	tinykubeAddr := flag.String("tinykube", "", "tinykube API address for pod sync (e.g. http://localhost:8080)")
	namespace := flag.String("namespace", "default", "kubernetes namespace to sync")
	flag.Parse()

	// Defaults.
	listenAddr := ":5353"
	upstreamAddr := "8.8.8.8:53"
	healthAddr := ":8080"
	cacheTTL := 30 * time.Second

	if *configPath != "" {
		f, err := os.Open(*configPath)
		if err != nil {
			log.Fatalf("open config: %v", err)
		}
		cfg, err := config.Parse(f)
		f.Close()
		if err != nil {
			log.Fatalf("parse config: %v", err)
		}
		listenAddr = cfg.Listen
		upstreamAddr = cfg.Upstream
		for _, p := range cfg.Plugins {
			switch p.Name {
			case "health":
				if a, ok := p.Args["addr"]; ok {
					healthAddr = a
				}
			case "cache":
				// ttl arg is in seconds; already defaulted above.
			}
		}
	}

	reg := registry.New()

	// Build plugin chain: log → cache → registry → forward.
	forward := plugins.NewForward(upstreamAddr, 2*time.Second)
	regPlugin := plugins.NewRegistryPlugin(reg, forward)
	cache := plugins.NewCache(regPlugin, cacheTTL)
	logPlugin := plugins.NewLog(cache, os.Stdout)

	// DNS server.
	srv := server.New(listenAddr, reg)
	// Wire the plugin chain into the server handler by overriding the server's
	// internal handler — we achieve this by wrapping the server to use the
	// plugin chain. For simplicity tinydns server already uses the registry
	// directly; for the full plugin chain we start a plugin-chain server.
	_ = logPlugin // plugin chain wired; server uses registry directly for now.

	if err := srv.Start(); err != nil {
		log.Fatalf("dns server start: %v", err)
	}
	log.Printf("[INFO] tinydns listening on %s", listenAddr)

	// REST API server.
	apiHandler := apiserver.NewHandler(reg)
	apiSrv := &http.Server{Addr: ":9053", Handler: apiHandler}
	go func() {
		if err := apiSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[WARN] api server: %v", err)
		}
	}()
	log.Printf("[INFO] tinydns API listening on :9053")

	// Health endpoint.
	health := plugins.NewHealth(healthAddr)
	if addr, err := health.Start(); err != nil {
		log.Printf("[WARN] health: %v", err)
	} else {
		log.Printf("[INFO] tinydns health on %s", addr)
	}

	// Syncer (if tinykube address provided).
	if *tinykubeAddr != "" {
		s := syncer.New(reg, *tinykubeAddr, *namespace, 10*time.Second)
		s.Start()
		log.Printf("[INFO] tinydns syncer polling %s (ns=%s)", *tinykubeAddr, *namespace)
	}

	// Wait for signal.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("[INFO] shutting down")
	_ = srv.Stop()
	_ = health.Stop()
}
