package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

func getEnvStr(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	return getEnvIntFirst(defaultVal, key)
}

func getEnvStrFirst(defaultVal string, keys ...string) string {
	for _, key := range keys {
		if val := os.Getenv(key); val != "" {
			return val
		}
	}
	return defaultVal
}

func getEnvIntFirst(defaultVal int, keys ...string) int {
	for _, key := range keys {
		if val := os.Getenv(key); val != "" {
			if i, err := strconv.Atoi(val); err == nil {
				return i
			}
		}
	}
	return defaultVal
}

const maxEncoderWorkers = 16

func clampWorkers(n int) int {
	if n < 1 {
		return 1
	}
	if n > maxEncoderWorkers {
		return maxEncoderWorkers
	}
	return n
}

type ServerConfig struct {
	Port       int    `yaml:"port"`
	ListenAddr string `yaml:"listen_addr"`
	TimeZone   string `yaml:"timezone"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Name     string `yaml:"name"`
}

type EncoderConfig struct {
	FFmpegPath string `yaml:"ffmpeg_path"`
	TempDir    string `yaml:"temp_dir"`
	Workers    int    `yaml:"workers"`
	MinFreeGB  int    `yaml:"min_free_gb"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
}

type AuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type NtfyConfig struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

type DiscordConfig struct {
	WebhookURL string `yaml:"webhook_url"`
}

type GotifyConfig struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

type NotificationsConfig struct {
	Events     []string      `yaml:"events"`
	WebhookURL string        `yaml:"webhook_url"`
	Ntfy       NtfyConfig    `yaml:"ntfy"`
	Discord    DiscordConfig `yaml:"discord"`
	Gotify     GotifyConfig  `yaml:"gotify"`
}

type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Auth          AuthConfig          `yaml:"auth"`
	Database      DatabaseConfig      `yaml:"database"`
	Encoder       EncoderConfig       `yaml:"encoder"`
	Logging       LoggingConfig       `yaml:"logging"`
	Notifications NotificationsConfig `yaml:"notifications"`
}

func LoadConfig(path string) (*Config, error) {
	var cfg Config

	// Try reading from file first (it's okay if it doesn't exist for env-only deployments)
	data, err := os.ReadFile(path)
	if err == nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("could not parse config file: %w", err)
		}
	}

	// Override with env variables
	cfg.Server.Port = getEnvIntFirst(cfg.Server.Port, "GOENCODE_PORT", "GOENCODE_SERVER_PORT")
	cfg.Server.ListenAddr = getEnvStrFirst(cfg.Server.ListenAddr, "GOENCODE_LISTEN_ADDR", "GOENCODE_SERVER_LISTEN")
	cfg.Auth.Username = getEnvStr("GOENCODE_AUTH_USER", cfg.Auth.Username)
	cfg.Auth.Password = getEnvStr("GOENCODE_AUTH_PASS", cfg.Auth.Password)
	cfg.Database.Host = getEnvStr("GOENCODE_DB_HOST", cfg.Database.Host)
	cfg.Database.Port = getEnvInt("GOENCODE_DB_PORT", cfg.Database.Port)
	cfg.Database.User = getEnvStr("GOENCODE_DB_USER", cfg.Database.User)
	cfg.Database.Password = getEnvStr("GOENCODE_DB_PASS", cfg.Database.Password)
	cfg.Database.Name = getEnvStr("GOENCODE_DB_NAME", cfg.Database.Name)
	cfg.Encoder.FFmpegPath = getEnvStr("GOENCODE_FFMPEG_PATH", cfg.Encoder.FFmpegPath)
	cfg.Encoder.TempDir = getEnvStr("GOENCODE_ENCODER_TEMP", cfg.Encoder.TempDir)
	cfg.Encoder.Workers = getEnvInt("GOENCODE_ENCODER_WORKERS", cfg.Encoder.Workers)
	cfg.Encoder.MinFreeGB = getEnvInt("GOENCODE_ENCODER_MIN_FREE_GB", cfg.Encoder.MinFreeGB)
	cfg.Logging.Level = getEnvStr("GOENCODE_LOG_LEVEL", cfg.Logging.Level)
	cfg.Notifications.WebhookURL = getEnvStr("GOENCODE_WEBHOOK_URL", cfg.Notifications.WebhookURL)
	cfg.Notifications.Ntfy.URL = getEnvStr("GOENCODE_NTFY_URL", cfg.Notifications.Ntfy.URL)
	cfg.Notifications.Ntfy.Token = getEnvStr("GOENCODE_NTFY_TOKEN", cfg.Notifications.Ntfy.Token)
	cfg.Notifications.Discord.WebhookURL = getEnvStr("GOENCODE_DISCORD_WEBHOOK_URL", cfg.Notifications.Discord.WebhookURL)
	cfg.Notifications.Gotify.URL = getEnvStr("GOENCODE_GOTIFY_URL", cfg.Notifications.Gotify.URL)
	cfg.Notifications.Gotify.Token = getEnvStr("GOENCODE_GOTIFY_TOKEN", cfg.Notifications.Gotify.Token)
	if events := os.Getenv("GOENCODE_NOTIFY_EVENTS"); events != "" {
		cfg.Notifications.Events = splitCSV(events)
	}

	// Defaults if missing entirely
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.ListenAddr == "" {
		cfg.Server.ListenAddr = "0.0.0.0"
	}
	if cfg.Database.Port == 0 {
		cfg.Database.Port = 3306
	}
	if cfg.Encoder.TempDir == "" {
		cfg.Encoder.TempDir = "/tmp/goencode"
	}
	cfg.Encoder.Workers = clampWorkers(cfg.Encoder.Workers)
	if cfg.Encoder.MinFreeGB <= 0 {
		cfg.Encoder.MinFreeGB = 5
	}

	if envTZ := os.Getenv("TZ"); envTZ != "" {
		cfg.Server.TimeZone = envTZ
	}

	// Apply timezone if set
	if cfg.Server.TimeZone != "" {
		os.Setenv("TZ", cfg.Server.TimeZone)
	}

	return &cfg, nil
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
