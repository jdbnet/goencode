package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"goencode/internal/config"
	"goencode/internal/db"
	"goencode/internal/logger"
	"goencode/internal/queue"
	"goencode/internal/watcher"
	"goencode/internal/web"
)

var Version string = "dev"

func main() {
	configPath := flag.String("config", "goencode.yaml", "Path to configuration file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := db.Init(&cfg.Database); err != nil {
		log.Fatalf("Database init failed: %v", err)
	}

	sseServer := web.NewSSEServer()
	
	// Initialize logger
	logger.Init(sseServer.Broadcast)

	qm := queue.NewManager(cfg.Encoder.FFmpegPath, cfg.Encoder.TempDir, cfg.Notifications.WebhookURL, sseServer.Broadcast)
	qm.Start()
	defer qm.Stop()

	wm, err := watcher.NewManager(qm)
	if err != nil {
		log.Fatalf("Watcher init failed: %v", err)
	}
	wm.Start()
	defer wm.Stop()

	server := web.NewServer(cfg, qm, wm, sseServer, Version)
	
	go func() {
		log.Printf("Starting GoEncode Web UI (v%s) on %s:%d", Version, cfg.Server.ListenAddr, cfg.Server.Port)
		if err := server.Start(); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("Shutting down...")
}
