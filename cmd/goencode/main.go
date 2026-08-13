package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"goencode/internal/config"
	"goencode/internal/db"
	"goencode/internal/logger"
	"goencode/internal/notify"
	"goencode/internal/queue"
	"goencode/internal/updater"
	"goencode/internal/watcher"
	"goencode/internal/web"
)

var Version string = "dev"

func main() {
	configPath := flag.String("config", "goencode.yaml", "Path to configuration file")
	noUpdate := flag.Bool("no-update", false, "Disable automatic update check on startup")
	flag.Parse()

	if err := updater.MaybeUpdate(Version, *noUpdate); err != nil {
		log.Printf("Update check failed, continuing: %v", err)
	}

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

	loc := time.Local
	if cfg.Server.TimeZone != "" {
		if loaded, err := time.LoadLocation(cfg.Server.TimeZone); err != nil {
			log.Printf("Invalid timezone %q, using local: %v", cfg.Server.TimeZone, err)
		} else {
			loc = loaded
		}
	}

	qm := queue.NewManager(cfg.Encoder.FFmpegPath, cfg.Encoder.TempDir, cfg.Encoder.Workers, loc, sseServer.Broadcast)
	qm.SetThreads(cfg.Encoder.Threads)
	qm.Notifier = notify.New(notify.Options{
		Events:         cfg.Notifications.Events,
		WebhookURL:     cfg.Notifications.WebhookURL,
		NtfyURL:        cfg.Notifications.Ntfy.URL,
		NtfyToken:      cfg.Notifications.Ntfy.Token,
		DiscordWebhook: cfg.Notifications.Discord.WebhookURL,
		GotifyURL:      cfg.Notifications.Gotify.URL,
		GotifyToken:    cfg.Notifications.Gotify.Token,
	})
	qm.MinFreeBytes = int64(cfg.Encoder.MinFreeGB) * 1024 * 1024 * 1024
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
