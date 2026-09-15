package config

import (
	"testing"
)

func TestListenAddrAndPortAliases(t *testing.T) {
	t.Setenv("GOENCODE_LISTEN_ADDR", "")
	t.Setenv("GOENCODE_PORT", "")
	t.Setenv("GOENCODE_SERVER_LISTEN", "127.0.0.1")
	t.Setenv("GOENCODE_SERVER_PORT", "9090")

	cfg, err := LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.ListenAddr != "127.0.0.1" {
		t.Fatalf("ListenAddr = %q, want 127.0.0.1", cfg.Server.ListenAddr)
	}
	if cfg.Server.Port != 9090 {
		t.Fatalf("Port = %d, want 9090", cfg.Server.Port)
	}
}

func TestCanonicalListenOverridesAlias(t *testing.T) {
	t.Setenv("GOENCODE_LISTEN_ADDR", "10.0.0.1")
	t.Setenv("GOENCODE_SERVER_LISTEN", "127.0.0.1")
	t.Setenv("GOENCODE_PORT", "8081")
	t.Setenv("GOENCODE_SERVER_PORT", "9090")

	cfg, err := LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.ListenAddr != "10.0.0.1" {
		t.Fatalf("ListenAddr = %q, want 10.0.0.1", cfg.Server.ListenAddr)
	}
	if cfg.Server.Port != 8081 {
		t.Fatalf("Port = %d, want 8081", cfg.Server.Port)
	}
}

func TestEncoderWorkersEnvAndClamp(t *testing.T) {
	t.Setenv("GOENCODE_ENCODER_WORKERS", "4")
	cfg, err := LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Encoder.Workers != 4 {
		t.Fatalf("Workers = %d, want 4", cfg.Encoder.Workers)
	}

	t.Setenv("GOENCODE_ENCODER_WORKERS", "99")
	cfg, err = LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Encoder.Workers != maxEncoderWorkers {
		t.Fatalf("Workers = %d, want %d", cfg.Encoder.Workers, maxEncoderWorkers)
	}
}

func TestEncoderWorkersDefault(t *testing.T) {
	t.Setenv("GOENCODE_ENCODER_WORKERS", "")
	cfg, err := LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Encoder.Workers != 1 {
		t.Fatalf("Workers = %d, want 1", cfg.Encoder.Workers)
	}
	if cfg.Encoder.MinFreeGB != 5 {
		t.Fatalf("MinFreeGB = %d, want 5", cfg.Encoder.MinFreeGB)
	}
	if cfg.Encoder.Threads != 0 {
		t.Fatalf("Threads = %d, want 0", cfg.Encoder.Threads)
	}
}

func TestEncoderThreadsEnvAndClamp(t *testing.T) {
	t.Setenv("GOENCODE_ENCODER_THREADS", "4")
	cfg, err := LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Encoder.Threads != 4 {
		t.Fatalf("Threads = %d, want 4", cfg.Encoder.Threads)
	}

	t.Setenv("GOENCODE_ENCODER_THREADS", "999")
	cfg, err = LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Encoder.Threads != 256 {
		t.Fatalf("Threads = %d, want 256", cfg.Encoder.Threads)
	}

	t.Setenv("GOENCODE_ENCODER_THREADS", "-2")
	cfg, err = LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Encoder.Threads != 0 {
		t.Fatalf("Threads = %d, want 0", cfg.Encoder.Threads)
	}
}

func TestNotifyEventsEnv(t *testing.T) {
	t.Setenv("GOENCODE_NOTIFY_EVENTS", "failed, success")
	t.Setenv("GOENCODE_NTFY_URL", "https://ntfy.sh/goencode")
	t.Setenv("GOENCODE_NTFY_TOKEN", "tk_test")
	t.Setenv("GOENCODE_DISCORD_WEBHOOK_URL", "https://discord.com/api/webhooks/1/abc")
	t.Setenv("GOENCODE_GOTIFY_URL", "https://gotify.example")
	t.Setenv("GOENCODE_GOTIFY_TOKEN", "app")

	cfg, err := LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Notifications.Events) != 2 || cfg.Notifications.Events[0] != "failed" || cfg.Notifications.Events[1] != "success" {
		t.Fatalf("Events = %#v", cfg.Notifications.Events)
	}
	if cfg.Notifications.Ntfy.URL != "https://ntfy.sh/goencode" || cfg.Notifications.Ntfy.Token != "tk_test" {
		t.Fatalf("ntfy = %+v", cfg.Notifications.Ntfy)
	}
	if cfg.Notifications.Discord.WebhookURL == "" || cfg.Notifications.Gotify.Token != "app" {
		t.Fatalf("discord/gotify = %+v %+v", cfg.Notifications.Discord, cfg.Notifications.Gotify)
	}
}

func TestDatabaseDefaultsToSQLite(t *testing.T) {
	cfg, err := LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("Driver = %q, want sqlite", cfg.Database.Driver)
	}
	if cfg.Database.Path != "goencode.db" {
		t.Fatalf("Path = %q, want goencode.db", cfg.Database.Path)
	}
	if cfg.Database.Port != 0 {
		t.Fatalf("Port = %d, want 0 for sqlite", cfg.Database.Port)
	}
}

func TestDatabaseHostImpliesMySQL(t *testing.T) {
	t.Setenv("GOENCODE_DB_HOST", "db")
	cfg, err := LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.Driver != "mysql" {
		t.Fatalf("Driver = %q, want mysql", cfg.Database.Driver)
	}
	if cfg.Database.Port != 3306 {
		t.Fatalf("Port = %d, want 3306", cfg.Database.Port)
	}
}

func TestDatabaseDriverSQLiteOverridesHost(t *testing.T) {
	t.Setenv("GOENCODE_DB_HOST", "db")
	t.Setenv("GOENCODE_DB_DRIVER", "sqlite")
	t.Setenv("GOENCODE_DB_PATH", "/var/lib/goencode/goencode.db")
	cfg, err := LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("Driver = %q, want sqlite", cfg.Database.Driver)
	}
	if cfg.Database.Path != "/var/lib/goencode/goencode.db" {
		t.Fatalf("Path = %q", cfg.Database.Path)
	}
}

func TestDownloaderWorkersDefault(t *testing.T) {
	t.Setenv("GOENCODE_DOWNLOADER_WORKERS", "")
	cfg, err := LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Downloader.Workers != 1 {
		t.Fatalf("Workers = %d, want 1", cfg.Downloader.Workers)
	}
	if cfg.Downloader.YTDLPPath != "yt-dlp" {
		t.Fatalf("YTDLPPath = %q, want yt-dlp", cfg.Downloader.YTDLPPath)
	}
}

func TestDownloaderWorkersEnv(t *testing.T) {
	t.Setenv("GOENCODE_DOWNLOADER_WORKERS", "3")
	t.Setenv("GOENCODE_YTDLP_PATH", "/usr/bin/yt-dlp")
	cfg, err := LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Downloader.Workers != 3 {
		t.Fatalf("Workers = %d, want 3", cfg.Downloader.Workers)
	}
	if cfg.Downloader.YTDLPPath != "/usr/bin/yt-dlp" {
		t.Fatalf("YTDLPPath = %q", cfg.Downloader.YTDLPPath)
	}
}

func TestDownloaderCookiesEnv(t *testing.T) {
	t.Setenv("GOENCODE_YTDLP_COOKIES_FILE", "/etc/goencode/cookies.txt")
	t.Setenv("GOENCODE_YTDLP_COOKIES_FROM_BROWSER", "firefox")
	cfg, err := LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Downloader.CookiesFile != "/etc/goencode/cookies.txt" {
		t.Fatalf("CookiesFile = %q", cfg.Downloader.CookiesFile)
	}
	if cfg.Downloader.CookiesFromBrowser != "firefox" {
		t.Fatalf("CookiesFromBrowser = %q", cfg.Downloader.CookiesFromBrowser)
	}
}

func TestDatabaseMariaDBAlias(t *testing.T) {
	t.Setenv("GOENCODE_DB_DRIVER", "mariadb")
	t.Setenv("GOENCODE_DB_HOST", "127.0.0.1")
	cfg, err := LoadConfig("does-not-exist.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database.Driver != "mysql" {
		t.Fatalf("Driver = %q, want mysql", cfg.Database.Driver)
	}
}
