package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/distributed_task_queue/distributed_task_queue/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	log.Printf("orchestrator: listen=%s reconcile=%s redis=%s",
		cfg.OrchestratorListen, cfg.ReconcileInterval, cfg.RedisAddr)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}
