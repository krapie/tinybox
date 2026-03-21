package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/krapi0314/tinybox/tinykube/apiserver"
	"github.com/krapi0314/tinybox/tinykube/controller"
	"github.com/krapi0314/tinybox/tinykube/runtime"
	"github.com/krapi0314/tinybox/tinykube/store"
)

func main() {
	log.Println("Starting tinykube...")

	// Create the in-memory store (etcd substitute).
	s := store.New()

	// Create the Docker runtime.
	rt, err := runtime.NewDockerRuntime()
	if err != nil {
		log.Fatalf("failed to create DockerRuntime: %v", err)
	}
	defer func() { _ = rt.Close() }()

	// Create and start the deployment controller.
	ctrl := controller.NewDeploymentController(s, rt)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ctrl.Start(ctx, 5*time.Second)
	log.Println("Deployment controller started (reconcileInterval=5s)")

	// Start the API server.
	srv := apiserver.New(s)
	addr := ":8080"
	log.Printf("API server listening on %s", addr)

	// Graceful shutdown on SIGINT/SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Shutting down...")
		cancel()
	}()

	if err := srv.ListenAndServe(addr); err != nil {
		log.Fatalf("API server error: %v", err)
	}
}
