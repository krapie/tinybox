package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/krapi0314/tinybox/tinykube/apiserver"
	"github.com/krapi0314/tinybox/tinykube/controller"
	"github.com/krapi0314/tinybox/tinykube/logger"
	"github.com/krapi0314/tinybox/tinykube/runtime"
	"github.com/krapi0314/tinybox/tinykube/store"
)

func main() {
	log := logger.New(true) // debug enabled by default
	log.Info("Starting tinykube...")

	// Create the in-memory store (etcd substitute).
	s := store.New(log)

	// Create the Docker runtime.
	rt, err := runtime.NewDockerRuntime(log)
	if err != nil {
		log.Info("failed to create DockerRuntime: %v", err)
		os.Exit(1)
	}
	defer func() { _ = rt.Close() }()

	// Create and start the deployment controller.
	ctrl := controller.NewDeploymentController(s, rt, log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ctrl.Start(ctx, 5*time.Second)
	log.Info("Deployment controller started (reconcileInterval=5s)")

	// Start the API server.
	srv := apiserver.New(s)
	addr := ":8080"
	log.Info("API server listening on %s", addr)

	// Graceful shutdown on SIGINT/SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Info("Shutting down...")
		cancel()
	}()

	if err := srv.ListenAndServe(addr); err != nil {
		log.Info("API server error: %v", err)
		os.Exit(1)
	}
}
